"""Three real peers of the history protocol: sender, recipient, signed server.

Disposable Docker networks separate the native peers while both retain their
cloud route. Stopping the test-owned proxy simulates cloud disconnection.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import ipaddress
import json
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--context", default="default")
    parser.add_argument("--api", default="http://127.0.0.1:18680")
    parser.add_argument("--output", required=True)
    parser.add_argument("--restart-api-container", default="", help="Explicit staging API container to restart during the cloud outage")
    args = parser.parse_args()
    docker = ["docker", "--context", args.context]
    suffix = uuid.uuid4().hex[:8]
    prefix = "pinelink-sync-"+suffix
    a, b, proxy = prefix+"-a", prefix+"-b", prefix+"-proxy"
    net_a, net_b = prefix+"-lan", prefix+"-partition"
    containers, networks = [], []
    checks = {}
    started_at = datetime.now(timezone.utc).isoformat()
    started = time.monotonic()

    def run(command, check=True, timeout=60):
        try:
            result = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", timeout=timeout)
        except subprocess.TimeoutExpired:
            raise RuntimeError("replication smoke command timed out") from None
        if check and result.returncode:
            raise RuntimeError("replication command failed: "+result.stderr[-400:])
        return result

    def d(*parts, **kwargs):
        return run(docker+list(parts), **kwargs)

    def cli(container, *parts, **kwargs):
        return d("exec", container, "pinelink", "--json", *parts, **kwargs)

    def value(container, *parts):
        output = cli(container, *parts).stdout
        return json.loads(output) if output.strip() else None

    def wait(read, predicate, timeout=100):
        deadline = time.monotonic()+timeout
        last = None
        while time.monotonic() < deadline:
            last = read()
            if predicate(last):
                return last
            time.sleep(0.4)
        raise RuntimeError("history convergence timed out")

    def ready(container):
        def read():
            result = d("exec", container, "cat", "/state/ready.json", check=False)
            return json.loads(result.stdout) if result.returncode == 0 else None
        return wait(read, lambda v: v is not None, 30)

    def cloud_status(container):
        return next((t for t in value(container, "lan", "sync", "status") if t["kind"] == "CLOUD"), {})

    def transfer(container, transfer_id):
        return next((t for t in value(container, "lan", "local", "transfers") if t["id"] == transfer_id), None)

    def history_state(container, transfer_id, state, source=None):
        return any(h["aggregate_id"] == transfer_id and h["fact"].get("state") == state and (source is None or h["source_root"] == source) for h in value(container, "lan", "sync", "history"))

    def send_with_consent(source, destination, filename, peer):
        offered = value(source, "lan", "send", filename, "--peer", peer, "--offer-only")
        value(destination, "lan", "accept", offered["id"])
        result = value(source, "lan", "resume-send", offered["id"])
        assert result["state"] == "COMPLETED" and result.get("receipt")
        return result

    def synced(container):
        status = cloud_status(container)
        local = value(container, "lan", "changes")
        return status.get("bootstrapped") and not status.get("last_error") and status.get("uploaded_sequence", -1) >= local["next_sequence"]

    try:
        nonce = secrets.token_urlsafe(32)
        with urllib.request.urlopen(args.api+"/v1/replication/info?nonce="+nonce, timeout=10) as response:
            server_info = json.load(response)
        server_key = server_info["public_key"]
        image = "pinelink-staging-peer:"+args.version
        existing_names = d("network", "ls", "--format", "{{.Name}}").stdout.splitlines()
        # A separate smoke may remove its owned network between ls and inspect.
        inspected = json.loads(d("network", "inspect", *existing_names, check=False).stdout)
        occupied = [ipaddress.ip_network(c["Subnet"]) for n in inspected for c in n["IPAM"]["Config"] or [] if c.get("Subnet")]
        candidates = [ipaddress.ip_network(f"10.250.{v}.0/24") for v in range(248, 235, -1)]
        selected = [n for n in candidates if not any(n.overlaps(e) for e in occupied if e.version == 4)][:2]
        assert len(selected) == 2
        subnet_a, subnet_b = map(str, selected)
        ip_a = str(selected[0].network_address).rsplit(".", 1)[0]
        ip_b = str(selected[1].network_address).rsplit(".", 1)[0]
        for name, subnet in [(net_a, subnet_a), (net_b, subnet_b)]:
            d("network", "create", "--internal", "--subnet", subnet, name)
            networks.append(name)
        d("create", "--name", proxy, "--network", net_a, "--ip", ip_a+".4", "nginx:stable-alpine")
        containers.append(proxy)
        d("network", "connect", "--ip", ip_b+".4", net_b, proxy)
        d("network", "connect", "pinelink-staging_default", proxy)
        with tempfile.TemporaryDirectory(prefix="pinelink-sync-fixture-") as temporary:
            directory = Path(temporary)
            config = directory/"default.conf"
            config.write_text("server { listen 8080; location / { proxy_pass "+args.api+"; proxy_read_timeout 30s; } }\n")
            d("cp", str(config), proxy+":/etc/nginx/conf.d/default.conf")
            d("start", proxy)
            for container, number in [(a, 2), (b, 3)]:
                d("run", "-d", "--name", container, "--network", net_a, "--ip", ip_a+f".{number}", image,
                  "--device", container, "--json", "lan", "serve", "--advertise", "https://"+ip_a+f".{number}:19091", "--receive-dir", "/received", "--ready-file", "/state/ready.json")
                containers.append(container)
            # The entrypoint flag does not change inherited env for docker exec.
            # Wrapper arguments explicitly select the same device on every call.
            d("network", "connect", "--ip", ip_b+".3", net_b, b)
            a_ready, b_ready = ready(a), ready(b)
            # Update CLI helper to use the service's explicit device selector.
            def cli(container, *parts, **kwargs):
                return d("exec", container, "pinelink", "--device", container, "--json", *parts, **kwargs)
            pairing = subprocess.Popen(docker+["exec", a, "pinelink", "--device", a, "--json", "lan", "pair", b_ready["connection_code"]], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")
            requests = wait(lambda: value(b, "lan", "local", "requests"), lambda v: bool(v), 30)
            value(b, "lan", "local", "approve", requests[0]["id"])
            paired_text, _ = pairing.communicate(timeout=30)
            assert pairing.returncode == 0
            paired = json.loads(paired_text)
            for container, proxy_ip in [(a, ip_a+".4"), (b, ip_b+".4")]:
                value(container, "lan", "sync", "bind", "--server", "http://"+proxy_ip+":8080", "--server-key", server_key, "--cloud-owner", "local", "--cloud-device", container, "--configure-only")
            wait(lambda: synced(a) and synced(b), bool)
            checks["signed_device_binding_and_mutual_history_grant"] = True

            fixture = directory/"private-history-fixture.fig"
            content = b"encrypted incremental history\n"*8192
            fixture.write_bytes(content)
            digest = hashlib.sha256(content).hexdigest()
            d("cp", str(fixture), a+":/state/private-history-fixture.fig")
            first = send_with_consent(a, b, "/state/private-history-fixture.fig", paired["id"])
            wait(lambda: synced(a) and synced(b), bool)
            checks["initial_three_party_convergence"] = True

            d("stop", "--time", "3", proxy)
            if args.restart_api_container:
                # The proxy stays down while real local changes continue.
                d("restart", "--time", "3", args.restart_api_container)
                def restarted_info():
                    try:
                        with urllib.request.urlopen(args.api+"/v1/replication/info?nonce="+nonce, timeout=3) as response:
                            return json.load(response)
                    except (OSError, ValueError):
                        return None
                after_restart = wait(restarted_info, lambda v: v is not None, 30)
                assert after_restart["epoch"] == server_info["epoch"] and after_restart["public_key"] == server_key
                checks["server_restart_preserves_epoch_and_signer"] = True
            second = send_with_consent(a, b, "/state/private-history-fixture.fig", paired["id"])
            pending = value(a, "lan", "send", "/state/private-history-fixture.fig", "--peer", paired["id"], "--offer-only")
            d("network", "disconnect", net_a, b)
            # B can still control itself using loopback with the LAN IP absent.
            assert value(b, "lan", "reject", pending["id"])["state"] == "REJECTED"
            queued_result = cli(a, "lan", "send", "/state/private-history-fixture.fig", "--peer", paired["id"], check=False)
            assert queued_result.returncode != 0 and "transfer_id=" in queued_result.stderr
            queued_id = queued_result.stderr.split("transfer_id=", 1)[1].splitlines()[0].split(":", 1)[0]
            d("restart", "--time", "3", b)
            ready(b)
            d("start", proxy)
            # A and B are on different internal networks; only the proxy is on
            # both. Successful history exchange here must use the signed cloud.
            wait(lambda: history_state(a, pending["id"], "REJECTED", b_ready["peer"]["id"]), bool)
            wait(lambda: history_state(b, queued_id, "PREPARING", a_ready["peer"]["id"]), bool)
            checks["cloud_outage_and_independent_offline_changes"] = True
            checks["cloud_only_catchup_with_peer_partition"] = True
            checks["recipient_restart_preserves_unpublished_changes"] = True

            d("network", "connect", "--ip", ip_a+".3", net_a, b)
            wait(lambda: transfer(b, queued_id), lambda t: t is not None)
            value(b, "lan", "accept", queued_id)
            wait(lambda: history_state(a, queued_id, "COMPLETED", b_ready["peer"]["id"]), bool)
            final = transfer(b, queued_id)
            actual = d("exec", b, "sha256sum", "/received/"+final["output_name"]).stdout.split()[0]
            assert actual == digest
            wait(lambda: synced(a) and synced(b), bool)
            checks["queued_file_request_and_bytes_resume_automatically"] = True

            # A real server-owned storage event must arrive as server authority,
            # separate from native delivery and opaque peer metadata.
            uploaded = value(a, "--server", "http://"+ip_a+".4:8080", "--scope", prefix, "--encryption-domain", prefix, "upload", "/state/private-history-fixture.fig")
            wait(lambda: any(h["kind"].startswith("SERVER_") for h in value(a, "lan", "sync", "history")), bool)
            checks["authoritative_server_facts_are_separate"] = True
            before = len(value(a, "lan", "sync", "history"))
            # Duplicate retries are safe and should not duplicate projections.
            value(a, "lan", "sync", "once")
            value(b, "lan", "sync", "once")
            assert len(value(a, "lan", "sync", "history")) == before
            checks["explicit_retry_does_not_duplicate_history"] = True
            statuses = {"a": value(a, "lan", "sync", "status"), "b": value(b, "lan", "sync", "status")}
            evidence = {"version": args.version, "started_at": started_at, "completed_at": datetime.now(timezone.utc).isoformat(), "duration_seconds": round(time.monotonic()-started, 3), "server": args.api, "server_epoch": server_info["epoch"], "server_public_key": server_key, "networks": {"initial": subnet_a, "partition": subnet_b}, "payload_bytes": len(content), "plaintext_sha256": digest, "checks": checks, "statuses": statuses, "scope": "native replication/data plane; Web/Flutter storage adapters are subsequent work"}
            output = Path(args.output).resolve()
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_text(json.dumps(evidence, indent=2)+"\n", encoding="utf-8")
            print(json.dumps(evidence, indent=2))
    finally:
        for container in reversed(containers):
            d("rm", "-f", "-v", container, check=False)
        for network in reversed(networks):
            d("network", "rm", network, check=False)


if __name__ == "__main__":
    main()
