import importlib.util
import pathlib
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET

spec=importlib.util.spec_from_file_location("installer",pathlib.Path(__file__).with_name("package_installer.py"))
installer=importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)

class InstallerTest(unittest.TestCase):
    def stage(self,root):
        stage=root/"stage"
        (stage/"bin").mkdir(parents=True)
        (stage/"bin/syndirelay.exe").write_bytes(b"fixture")
        (stage/"bin/syndirelay-open.exe").write_bytes(b"fixture")
        (stage/"bin/node.exe").write_bytes(b"fixture")
        (stage/"package.json").write_text('{"name":"syndirelay","version":"1.2.3"}')
        (stage/"package-lock.json").write_text('{"version":"1.2.3","packages":{"":{"version":"1.2.3"}}}')
        (stage/"renderers").mkdir()
        (stage/"node_modules").mkdir()
        (stage/"scripts").mkdir()
        (stage/"THIRD_PARTY_LICENSES").mkdir()
        (stage/"THIRD_PARTY_LICENSES/MANIFEST.json").write_text('{"schema":1,"dependencies":[]}')
        (stage/"THIRD_PARTY_LICENSES/runtime/node").mkdir(parents=True)
        (stage/"THIRD_PARTY_LICENSES/runtime/node/LICENSE").write_text("Node.js upstream license fixture")
        (stage/"LICENSE").write_text("test")
        (stage/"Open-SyndiRelay.sh").write_text("test")
        (stage/"extension/icons").mkdir(parents=True)
        (stage/"extension/icons/icon128.png").write_bytes(b"fixture-icon")
        (stage/"extension/manifest.json").write_text('{"version":"1.2.3"}')

        return stage

    def test_layout_preserves_unmanaged_state(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp);stage=self.stage(root)
            installer.prepare(stage,root/"out","1.2.3","windows")
            config=ET.parse(root/"out/config/config.xml").getroot()
            self.assertEqual(config.findtext("RemoveTargetDir"),"false")
            data=root/"out/packages/com.thinkerqaq.syndirelay/data"
            self.assertTrue((data/"bin/syndirelay.exe").exists())
            self.assertTrue((data/"bin/node.exe").exists())
            self.assertFalse((data/"config.toml").exists())
            self.assertFalse((data/"Data").exists())
            script=(root/"out/packages/com.thinkerqaq.syndirelay/meta/installscript.qs").read_text()
            self.assertIn("syndirelay-open.exe",script)
            self.assertNotIn("WindowsPowerShell",script)
            self.assertIn("UserStartMenuProgramsPath",script)
            self.assertNotIn("addStopProcessForUpdateRequest",script)

    def test_rejects_mismatched_version(self):
        for filename in ("extension/manifest.json", "package.json", "package-lock.json"):
            with self.subTest(filename=filename), tempfile.TemporaryDirectory() as temp:
                root=pathlib.Path(temp);stage=self.stage(root)
                (stage/filename).write_text('{"version":"0.0.1"}')
                with self.assertRaises(ValueError):
                    installer.prepare(stage,root/"out","1.2.3","windows")

    def test_unix_launch_entries(self):
        for target in ("linux","darwin"):
            with self.subTest(target=target),tempfile.TemporaryDirectory() as temp:
                root=pathlib.Path(temp);stage=self.stage(root)
                (stage/"bin/syndirelay.exe").rename(stage/"bin/syndirelay")
                (stage/"bin/node.exe").rename(stage/"bin/node")
                installer.prepare(stage,root/"out","1.2.3",target)
                if target=="linux":
                    script=(root/"out/packages/com.thinkerqaq.syndirelay/meta/installscript.qs").read_text()
                    self.assertIn("CreateDesktopEntry",script)
                else:
                    launcher=root/"out/packages/com.thinkerqaq.syndirelay/data/SyndiRelay.app/Contents/MacOS/SyndiRelay"
                    self.assertTrue(launcher.is_file())
                    self.assertIn("Open-SyndiRelay.sh",launcher.read_text())

    def test_single_product_wizard_and_finish_choices(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp);stage=self.stage(root)
            installer.prepare(stage,root/"out","1.2.3","windows")
            config=ET.parse(root/"out/config/config.xml").getroot()
            self.assertEqual(config.findtext("Title"),"SyndiRelay")
            self.assertEqual(config.findtext("WizardShowPageList"),"false")
            self.assertEqual(config.findtext("ControlScript"),"controller.qs")
            self.assertEqual(config.findtext("StyleSheet"),"style.qss")
            self.assertEqual(config.findtext("InstallerApplicationIcon"),"syndirelay-application")
            self.assertTrue((root/"out/config/syndirelay-application.ico").is_file())
            for name in ("Logo","InstallerWindowIcon"):
                filename=config.findtext(name)
                if name=="InstallerWindowIcon": filename += ".png"
                self.assertTrue((root/"out/config"/filename).is_file())
            meta=root/"out/packages/com.thinkerqaq.syndirelay/meta"
            package=ET.parse(meta/"package.xml").getroot()
            self.assertEqual(package.find("Licenses/License").get("file"),"license.txt")
            self.assertEqual((meta/"license.txt").read_bytes(),(stage/"LICENSE").read_bytes())
            ui=ET.parse(meta/"first-run.ui").getroot()
            checks={w.get("name"):w.findtext("property[@name='checked']/bool") for w in ui.findall(".//widget[@class='QCheckBox']")}
            self.assertEqual(checks,{"openAppCheckBox":"false","openExtensionCheckBox":"false","openReadmeCheckBox":"false"})
            data=root/"out/packages/com.thinkerqaq.syndirelay/data"
            self.assertFalse(list(data.rglob("*.ps1")))
            self.assertTrue((data/"THIRD_PARTY_LICENSES/runtime/node/LICENSE").exists())

    def test_qa_stage_not_distributable(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp); stage=self.stage(root)
            (stage/".qa-only").write_text("QA only")
            installer.QA_PREPARE_ONLY=False
            with self.assertRaises(ValueError):
                installer.prepare(stage, root/"release", "1.2.3", "windows")
            installer.QA_PREPARE_ONLY=True
            try:
                installer.prepare(stage, root/"qa", "1.2.3", "windows")
            finally:
                installer.QA_PREPARE_ONLY=False

    def test_local_install_test_mode_marks_package_and_preserves_release_gate(self):
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp); stage=self.stage(root)
            (stage/".qa-only").write_text("local QA build")
            (stage/"THIRD_PARTY_LICENSES/MANIFEST.json").unlink()
            installer.QA_PREPARE_ONLY=False
            installer.LOCAL_TEST_ONLY=False
            with self.assertRaises(ValueError):
                installer.prepare(stage,root/"release","1.2.3","windows")
            installer.LOCAL_TEST_ONLY=True
            try:
                installer.prepare(stage,root/"local-test","1.2.3","windows")
            finally:
                installer.LOCAL_TEST_ONLY=False
            data=root/"local-test/packages/com.thinkerqaq.syndirelay/data"
            self.assertTrue((data/"LOCAL-TEST-ONLY.txt").is_file())
            self.assertFalse((data/"Data").exists())

    def test_rejects_private_state_and_symlinks(self):
        for name in ("Data/state.sqlite","config.toml","scripts/runtime.env","extension/extension-access.key","scripts/legacy.ps1"):
            with self.subTest(name=name),tempfile.TemporaryDirectory() as temp:
                root=pathlib.Path(temp);stage=self.stage(root)
                secret=stage/name;secret.parent.mkdir(parents=True,exist_ok=True);secret.write_text("private")
                with self.assertRaises(ValueError):
                    installer.prepare(stage,root/"out","1.2.3","windows")
        with tempfile.TemporaryDirectory() as temp:
            root=pathlib.Path(temp);stage=self.stage(root)
            try:
                (stage/"bin/link").symlink_to(stage/"LICENSE")
            except OSError as error:
                if getattr(error,"winerror",None)==1314:
                    self.skipTest("Windows symlink privilege is unavailable")
                raise
            with self.assertRaises(ValueError):
                installer.prepare(stage,root/"out","1.2.3","windows")

if __name__=="__main__":
    unittest.main()
