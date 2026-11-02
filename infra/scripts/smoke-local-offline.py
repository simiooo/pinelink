"""Exercise real native peers on a disposable, WAN-isolated Docker network.

No center API, DNS, STUN, TURN or proxy participates. Never persist invitation
tokens, keys or envelopes in the evidence file. Docker resources are uniquely
named and only resources created by this invocation are removed.
"""
import argparse
import hashlib
import ipaddress
import json
from pathlib import Path
import subprocess
import tempfile
import time
import uuid
from datetime import datetime, timezone


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--context", default="default")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    if len(args.version) != 40 or any(c not in "0123456789abcdef" for c in args.version):
        raise ValueError("version must be a full source commit")
    docker = ["docker", "--context", args.context]
    image = "pinelink-staging-peer:" + args.version
    suffix = uuid.uuid4().hex[:10]
    network = "pinelink-offline-" + suffix
    a, b = network + "-a", network + "-b"
    containers = []
    created_network = False
    started_at = datetime.now(timezone.utc).isoformat()
    started = time.monotonic()
    checks = {}

    def execute(command, *, check=True, timeout=30):
        try:
            result = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", timeout=timeout)
        except subprocess.TimeoutExpired:
            raise RuntimeError("isolated peer command timed out") from None
        if check and result.returncode:
            # Do not include command arguments: pairing arguments contain tokens.
            raise RuntimeError("peer command failed: " + result.stderr[-500:])
        return result

    def d(*parts, **kwargs):
        return execute(docker + list(parts), **kwargs)

    def cli(container, *parts, **kwargs):
        return d("exec", container, "pinelink", "--json", "lan", *parts, **kwargs)

    def value(container, *parts):
        output = cli(container, *parts).stdout
        return json.loads(output) if output.strip() else None

    def wait_for(read, predicate, timeout=30):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            item = read()
            if predicate(item):
                return item
            time.sleep(0.12)
        raise RuntimeError("expected local state was not reached")

    def ready(container):
        def read():
            result = d("exec", container, "cat", "/state/ready.json", check=False)
            return json.loads(result.stdout) if result.returncode == 0 else None
        return wait_for(read, lambda item: item is not None)

    def process(container, *parts):
        return subprocess.Popen(docker + ["exec", container, "pinelink", "--json", "lan", *parts], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8")

    def finish(proc, timeout=45, success=True):
        try:
            stdout, stderr = proc.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.communicate()
            raise RuntimeError("local foreground operation did not finish") from None
        if success and proc.returncode:
            raise RuntimeError("local foreground operation failed: " + stderr[-300:])
        return json.loads(stdout) if proc.returncode == 0 else None

    def transfer(container, transfer_id):
        return next(item for item in value(container, "local", "transfers") if item["id"] == transfer_id)

    def assert_delivered(result, destination, expected_hash):
        assert result["state"] == "COMPLETED" and result.get("receipt"), "missing verified delivery"
        assert result["route"] == "DIRECT" and result["spool_state"] == "NOT_REQUIRED", "unexpected cloud dependence"
        assert result["acknowledged_bytes"] == result["size_bytes"], "incomplete durable acknowledgement"
        received_hash = d("exec", destination, "sha256sum", "/received/" + result["output_name"]).stdout.split()[0]
        assert received_hash == expected_hash, "received plaintext differs"

    try:
        names = d("network", "ls", "--format", "{{.Name}}").stdout.splitlines()
        configs = json.loads(d("network", "inspect", *names).stdout)
        occupied = [ipaddress.ip_network(item["Subnet"]) for net in configs for item in net["IPAM"]["Config"] or [] if item.get("Subnet")]
        subnet = next(ipaddress.ip_network(f"10.251.{index}.0/24") for index in range(248, 239, -1) if not any(ipaddress.ip_network(f"10.251.{index}.0/24").overlaps(item) for item in occupied if item.version == 4))
        prefix = str(subnet.network_address).rsplit(".", 1)[0]
        ipv6 = "fd68:7069:6e65:848::/64"
        assert not any(ipaddress.ip_network(ipv6).overlaps(item) for item in occupied if item.version == 6), "test IPv6 subnet already in use"
        d("network", "create", "--internal", "--ipv6", "--subnet", str(subnet), "--subnet", ipv6, network)
        created_network = True
        inspection = json.loads(d("network", "inspect", network).stdout)[0]
        assert inspection["Internal"], "network is not isolated"
        checks["internal_network"] = True
        for container, number in [(a, 2), (b, 3)]:
            manual_only = ["--no-discovery"] if container == a else []
            # Keep a real partial checkpoint observable on fast loopback / SSD
            # hosts; constrain only this disposable sender, never the host.
            cpu_limit = ["--cpus", "0.10"] if container == a else []
            d("run", "-d", "--name", container, "--network", network,
              "--ip", f"{prefix}.{number}", "--ip6", f"fd68:7069:6e65:848::{number}",
              "--dns", "203.0.113.53", "-e", "PINELINK_DEVICE=" + container,
              "-e", "PINELINK_SERVER=http://203.0.113.1:1", "-e", "HTTPS_PROXY=http://127.0.0.1:1", "-e", "HTTP_PROXY=http://127.0.0.1:1", "-e", "ALL_PROXY=http://127.0.0.1:1",
              "-e", "PINELINK_STUN_URL=stun:203.0.113.1:1", "-e", "PINELINK_TURN_URL=turn:203.0.113.1:1",
              *cpu_limit, image, "--json", "lan", "serve", "--advertise", f"https://{prefix}.{number}:19091", "--receive-dir", "/received", "--ready-file", "/state/ready.json", *manual_only)
            containers.append(container)
            assert d("exec", container, "pinelink", "--version").stdout.strip() == "pinelink version " + args.version
            assert d("exec", container, "wget", "-q", "-T", "1", "-O", "/dev/null", "http://203.0.113.1:1/healthz", check=False).returncode != 0
            assert d("exec", container, "timeout", "2", "nslookup", "pinelink.invalid", "203.0.113.53", check=False).returncode != 0
        a_ready, b_ready = ready(a), ready(b)
        checks["no_center_or_external_dns"] = True
        checks["no_proxy_stun_turn_required"] = True
        nearby = value(a, "discover", "--timeout", "4s")
        assert any(item["id"] == b_ready["peer"]["id"] for item in nearby), "mDNS did not discover the other isolated peer"
        checks["mdns_discovery"] = True

        pairing = process(a, "pair", b_ready["connection_code"])
        requests = wait_for(lambda: value(b, "local", "requests"), lambda items: len(items) == 1)
        request = requests[0]
        assert request["state"] == "WAITING" and len(request["code"].replace(" ", "")) == 6, "pairing skipped user consent"
        value(b, "local", "approve", request["id"])
        paired = finish(pairing)
        assert paired["id"] == b_ready["peer"]["id"]
        checks["manual_one_time_pairing_and_confirmation"] = True

        with tempfile.TemporaryDirectory(prefix="pinelink-fixture-") as directory:
            fixture = Path(directory) / "Design source.fig"
            block = bytes(range(256)) * 4096
            with fixture.open("wb") as output:
                for _ in range(32):
                    output.write(block)
            expected_hash = hashlib.sha256(fixture.read_bytes()).hexdigest()
            d("cp", str(fixture), a + ":/state/input.fig")
            # Docker cp files are owned by root but intentionally world-readable;
            # only synthetic fixture bytes are copied into the peer.
            offered = value(a, "send", "/state/input.fig", "--peer", paired["id"], "--offer-only")
            current = transfer(b, offered["id"])
            assert current["state"] == "PENDING" and current["acknowledged_bytes"] == 0
            assert not d("exec", b, "ls", "-A", "/received").stdout.strip()
            checks["no_bytes_before_receive_consent"] = True
            value(b, "accept", offered["id"])
            sending = process(a, "resume-send", offered["id"])
            acknowledged = wait_for(lambda: transfer(b, offered["id"]), lambda item: item["acknowledged_bytes"] > 0)
            paused = value(a, "pause", offered["id"])
            assert paused["state"] == "PAUSED" and paused["acknowledged_bytes"] >= acknowledged["acknowledged_bytes"]
            # Kill only this test's foreground sender; the receiver service on
            # A stays alive. Its UUID-scoped cache lock is released by the OS.
            killed = d("exec", a, "pkill", "-KILL", "-f", "^pinelink --json lan resume-send " + offered["id"] + "$", check=False)
            if killed.returncode not in (0, 1):
                raise RuntimeError("could not interrupt the owned foreground sender")
            finish(sending, success=False)
            d("restart", "--time", "5", b)
            ready(b)
            restored = transfer(b, offered["id"])
            assert restored["state"] == "PAUSED" and restored["acknowledged_bytes"] == paused["acknowledged_bytes"], "receiver lost its durable checkpoint"
            value(b, "resume", offered["id"])
            completed = value(a, "resume-send", offered["id"])
            assert_delivered(completed, b, expected_hash)
            checks["pause_and_receiver_restart_resume"] = True
            checks["sender_interrupted_before_resume"] = killed.returncode == 0
            checks["encrypted_32mib_delivery_and_signed_receipt"] = True

            # One confirmed pairing grants reciprocal trust. IPv6 uses the same
            # pinned key and no multicast result / DNS / central signaling.
            d("cp", str(fixture), b + ":/state/reverse.fig")
            reverse = value(b, "send", "/state/reverse.fig", "--peer", a_ready["peer"]["id"], "--address", "https://[fd68:7069:6e65:848::2]:19091", "--offer-only")
            value(a, "accept", reverse["id"])
            reversed_result = value(b, "resume-send", reverse["id"], "--address", "https://[fd68:7069:6e65:848::2]:19091")
            assert_delivered(reversed_result, a, expected_hash)
            checks["reciprocal_ipv6_manual_direct_delivery"] = True

            empty = Path(directory) / "empty.txt"
            empty.write_bytes(b"")
            d("cp", str(empty), a + ":/state/empty.txt")
            empty_offer = value(a, "send", "/state/empty.txt", "--peer", paired["id"], "--offer-only")
            value(b, "accept", empty_offer["id"])
            empty_result = value(a, "resume-send", empty_offer["id"])
            assert_delivered(empty_result, b, hashlib.sha256(b"").hexdigest())
            checks["authenticated_empty_file"] = True

            rejected = value(a, "send", "/state/empty.txt", "--peer", paired["id"], "--offer-only")
            assert value(b, "reject", rejected["id"])["state"] == "REJECTED"
            assert cli(a, "resume-send", rejected["id"], check=False).returncode != 0
            cancelled = value(a, "send", "/state/empty.txt", "--peer", paired["id"], "--offer-only")
            assert value(a, "cancel", cancelled["id"])["state"] == "CANCELLED"
            assert transfer(b, cancelled["id"])["state"] == "CANCELLED"
            checks["reject_and_sender_cancel"] = True

        storage = {"a": value(a,"storage"), "b": value(b,"storage")}
        for info in storage.values():
            assert info["schema_version"] == 2 and info["journal_mode"] == "wal" and info["synchronous"] == 2 and info["integrity"] == "ok"
            assert info["inbox_facts"] >= 1, "verified receipt was not deduplicated in the local inbox"
        first_changes = value(a,"changes")
        assert first_changes["items"] and not value(a,"changes","--after",str(first_changes["next_sequence"]),"--stream",first_changes["stream_id"])["items"]
        checks["sqlite_wal_integrity_and_incremental_cursor"] = True

        d("stop", "--time", "5", b)
        history = value(a, "history")
        assert any(item["id"] == completed["id"] and item.get("receipt") for item in history)
        assert value(a, "resume-send", completed["id"])["state"] == "COMPLETED"
        checks["local_history_and_receipt_with_peer_offline"] = True
        value(a, "local", "forget", b_ready["peer"]["id"])
        assert cli(a, "send", "/state/empty.txt", "--peer", paired["id"], "--offer-only", check=False).returncode != 0
        checks["persistent_trust_revocation"] = True

        evidence = {"version": args.version, "started_at": started_at, "completed_at": datetime.now(timezone.utc).isoformat(), "duration_seconds": round(time.monotonic()-started, 3), "platform": "Fedora Docker / Linux amd64 native peers", "network": {"internal": True, "ipv4": str(subnet), "ipv6": ipv6, "published_ports": []}, "payload_bytes": 32*1024*1024, "plaintext_sha256": expected_hash, "checks": checks, "storage":storage, "scope": "native protocol/CLI; browser, Flutter, BLE and cloud delta replication are not covered"}
        path = Path(args.output).resolve()
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(evidence, indent=2)+"\n", encoding="utf-8")
        print(json.dumps(evidence, indent=2))
    finally:
        for container in reversed(containers):
            d("rm", "-f", "-v", container, check=False)
        if created_network:
            d("network", "rm", network, check=False)


if __name__ == "__main__":
    main()
