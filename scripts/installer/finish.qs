// Adapted from VidOtter's successful-installation hooks; see third_party/vidotter.
Component.prototype.showFirstRunOptions = function() {
    if (installer.isInstaller() && !installer.isCommandLineInstance() &&
        installer.status == QInstaller.Success) {
        installer.addWizardPageItem(component, "SyndiRelayFirstRun", QInstaller.InstallationFinished);
    }
};
Component.prototype.finishFirstRun = function() {
    if (!installer.isInstaller() || installer.isCommandLineInstance() ||
        installer.status != QInstaller.Success) return;
    var form = component.userInterface("SyndiRelayFirstRun");
    if (!form) return;
    var target = installer.value("TargetDir");
    console.log("SYNDIRELAY_FINISH_OPTIONS");
    if ("__PLATFORM__" == "windows") {
        var powershell = installer.environmentVariable("SystemRoot") + "/System32/WindowsPowerShell/v1.0/powershell.exe";
        if (form.openAppCheckBox.checked) {
            installer.executeDetached(powershell, ["-NoProfile", "-ExecutionPolicy", "Bypass",
                "-WindowStyle", "Hidden", "-File", installer.toNativeSeparators(target + "/Open-SyndiRelay.ps1")], target);
        }
        if (form.openExtensionCheckBox.checked) {
            installer.executeDetached(powershell, ["-NoProfile", "-ExecutionPolicy", "Bypass",
                "-WindowStyle", "Hidden", "-File", installer.toNativeSeparators(target + "/scripts/SyndiRelay-FirstRun.ps1")], target);
        }
    } else {
        if (form.openAppCheckBox.checked) {
            installer.executeDetached("/bin/bash", [target + "/Open-SyndiRelay.sh"], target);
        }
        if (form.openExtensionCheckBox.checked) {
            QDesktopServices.openUrl("file://" + target + "/extension");
            QDesktopServices.openUrl("file://" + target + "/README.md");
        }
    }
};
