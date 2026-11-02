"""Real Windows sender -> deployed Fedora native peer, with explicit consent."""
import argparse
import atexit
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import uuid


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--container", required=True)
    parser.add_argument("--context", default="default")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    output = Path(args.output).resolve()
    work = output.parent / ("windows-smoke-" + uuid.uuid4().hex[:8])
    work.mkdir(parents=True)
    marker = work.name
    environment = dict(os.environ, PINELINK_CONFIG_HOME=str(work / "profile"), PINELINK_OWNER="offline-smoke", PINELINK_DEVICE=marker, PINELINK_SERVER="http://203.0.113.1:1", HTTP_PROXY="http://127.0.0.1:1", HTTPS_PROXY="http://127.0.0.1:1")
    hidden = subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0
    binary = str(Path(args.binary).resolve())
    docker = ["docker", "--context", args.context, "exec", args.container]

    def run(command, env=None, timeout=30):
        result = subprocess.run(command, env=env, capture_output=True, text=True, encoding="utf-8", timeout=timeout, creationflags=hidden)
        if result.returncode:
            raise RuntimeError("native smoke command failed: " + result.stderr[-300:])
        return result.stdout

    def native(*parts):
        return [binary, "--json", "lan", *parts]

    def remote(*parts):
        output = run(docker + ["pinelink", "--json", "lan", *parts])
        return json.loads(output) if output.strip() else None

    def wait_for(read, predicate):
        for _ in range(80):
            value = read()
            if predicate(value):
                return value
            time.sleep(0.15)
        raise RuntimeError("native smoke did not reach the expected state")

    def finish(process):
        try:
            stdout, stderr = process.communicate(timeout=45)
        except subprocess.TimeoutExpired:
            process.kill()
            process.communicate()
            raise RuntimeError("native foreground operation timed out") from None
        if process.returncode:
            raise RuntimeError("native foreground operation failed: " + stderr[-300:])
        return json.loads(stdout), stderr

    def stop(process):
        if process.poll() is None:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()

    assert run([binary, "--version"], environment).strip() == "pinelink version " + args.version
    invitation = remote("local", "invite")["connection_code"]
    pairing = subprocess.Popen(native("pair", invitation, "--name", marker), env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", creationflags=hidden)
    atexit.register(stop, pairing)
    requests = wait_for(lambda: remote("local", "requests"), lambda items: any(item["peer"]["name"] == marker for item in items))
    request = next(item for item in requests if item["peer"]["name"] == marker)
    remote("local", "approve", request["id"])
    peer, pairing_stderr = finish(pairing)
    assert ("compare_code=" + request["code"]) in pairing_stderr, "pairing codes differ"
    fixture = work / (marker + ".txt")
    content = ("Windows to Fedora, without center service.\n"*8192).encode()
    fixture.write_bytes(content)
    sending = subprocess.Popen(native("send", str(fixture), "--peer", peer["id"]), env=environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", creationflags=hidden)
    atexit.register(stop, sending)
    transfers = wait_for(lambda: remote("local", "transfers"), lambda items: any(item["file_name"] == fixture.name for item in items))
    offered = next(item for item in transfers if item["file_name"] == fixture.name)
    assert offered["state"] == "PENDING" and offered["acknowledged_bytes"] == 0
    remote("accept", offered["id"])
    completed, _ = finish(sending)
    assert completed["state"] == "COMPLETED" and completed.get("receipt")
    expected = hashlib.sha256(content).hexdigest()
    actual = run(docker + ["sha256sum", "/received/"+completed["output_name"]]).split()[0]
    assert actual == expected
    remote("local", "forget", request["peer"]["id"])
    history = json.loads(run(native("history"), environment))
    assert any(item["id"] == completed["id"] and item.get("receipt") for item in history)
    storage = json.loads(run(native("storage"), environment))
    assert storage["journal_mode"] == "wal" and storage["synchronous"] == 2 and storage["integrity"] == "ok" and storage["inbox_facts"] == 1
    evidence = {"version": args.version, "completed_at": datetime.now(timezone.utc).isoformat(), "platform": "Windows native sender -> Fedora staging native peer", "endpoint": peer["address"], "payload_bytes": len(content), "plaintext_sha256": expected, "checks": {"manual_pairing_and_code_comparison": True, "receive_consent": True, "e2e_plaintext_hash": True, "signed_receipt": True, "local_history_after_revocation": True, "center_and_proxy_unavailable_to_sender": True}, "scope": "native CLI; Windows host WAN is not disconnected, isolated-network evidence is separate"}
    output.write_text(json.dumps(evidence, indent=2)+"\n", encoding="utf-8")
    print(json.dumps(evidence, indent=2))


if __name__ == "__main__":
    main()
