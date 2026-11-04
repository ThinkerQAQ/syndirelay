#!/usr/bin/env python3
"""Isolated Go-native Tunnel lifecycle smoke test. Never reads user credentials."""
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time

REPO = Path(__file__).resolve().parent.parent
def wait(predicate, label, seconds=9):
    deadline=time.monotonic()+seconds
    while time.monotonic()<deadline:
        result=predicate()
        if result:return result
        time.sleep(.12)
    raise AssertionError("Timed out waiting for "+label)

def read_json(path):
    try:return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):return {}

def atomic_json(path, value):
    tmp=path.with_suffix(".pending")
    tmp.write_text(json.dumps(value),encoding="utf-8")
    os.chmod(tmp,0o600)
    os.replace(tmp,path)

def live(pid):
    try:os.kill(pid,0);return True
    except ProcessLookupError:return False

def command(private, action):
    owner=read_json(private/"runner.json")
    assert owner.get("token") and owner.get("pid"),"runner never started"
    action_id=secrets.token_hex(16)
    atomic_json(private/"runner-command.json",{"token":owner["token"],"action":action,"id":action_id})
    state=wait(lambda: (lambda s:s if s.get("action_id")==action_id else None)(read_json(private/"runner-state.json")),action+" ack")
    assert state.get("connecting")==(action=="connect"),"unexpected connecting state"

def main():
    with tempfile.TemporaryDirectory(prefix="syndirelay-tunnel-qa-") as td:
        d=Path(td)
        executable=d/"syndirelay"
        subprocess.run(["go","build","-o",str(executable),"./cmd"],cwd=REPO,check=True,stdout=subprocess.DEVNULL)
        fake_bin=d/"bin"
        fake_bin.mkdir()
        fake_client=fake_bin/"tunnel-client"
        fake_client.write_text('#!/bin/sh\nif [ "$1" != "run" ]; then exit 12; fi\nprintf "%s\\n" "$$" > "$MOCK_TUNNEL_PID"\nwhile true; do sleep 2; done\n',encoding="utf-8")
        fake_client.chmod(0o700)
        cfg=d/"config"
        data=d/"Data"
        private=data/"mcp"
        profile=private/"syndirelay.yaml"
        private.mkdir(parents=True)
        profile.write_text("tunnel_id: tunnel_1234567890abcdef1234567890abcdef\n",encoding="utf-8")
        (private/"runtime.env").write_text("synthetic_only_runtime_key_1234567890\n",encoding="utf-8")
        atomic_json(private/"auto-connect.json",{"auto_connect":False})
        pid_file=d/"fake-tunnel.pid"
        env=os.environ.copy()
        env.update({"HOME":str(d),"XDG_CONFIG_HOME":str(cfg),"SYNDIRELAY_DATA_DIR":str(data),
                    "PATH":str(fake_bin)+os.pathsep+env.get("PATH",""),
                    "MOCK_TUNNEL_PID":str(pid_file)})
        process=subprocess.Popen([str(executable),"tunnel-run"],env=env,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        try:
            standby=wait(lambda: (lambda x:x if x.get("token") else None)(read_json(private/"runner-state.json")),"standby runner")
            assert standby["connecting"] is False,"auto-connect off should start standby"
            assert process.poll() is None,"supervisor stopped unexpectedly"
            assert not pid_file.exists(),"standby started Tunnel unexpectedly"
            active_profiles=list(private.glob("active-*.yaml"))
            assert len(active_profiles)==1,"supervisor did not snapshot its startup configuration"
            original=active_profiles[0].read_text(encoding="utf-8")
            # Simulate Save of a newer profile while the old supervisor is in standby.
            profile.write_text("tunnel_id: tunnel_abcdef1234567890abcdef1234567890\n",encoding="utf-8")
            assert active_profiles[0].read_text(encoding="utf-8")==original,"old runtime profile changed during Save"
            command(private,"connect")
            child_pid=int(wait(lambda:pid_file.read_text().strip() if pid_file.exists() else None,"owned Tunnel child"))
            assert live(child_pid),"Tunnel child exited on connect"
            assert process.poll() is None,"supervisor exited after connect"
            command(private,"disconnect")
            wait(lambda:not live(child_pid),"owned Tunnel child shutdown")
            assert process.poll() is None,"disconnect stopped local supervisor"
            info=read_json(private/"runner.json")
            (private/"stop-request").write_text(info["token"],encoding="utf-8")
            process.wait(timeout=8)
            assert process.returncode==0,"local supervisor returned error"
            print("PASS isolated Go Tunnel lifecycle: standby -> connect -> disconnect -> stop")
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=8)

if __name__=="__main__":
    main()
