#!/usr/bin/env python3
"""Opt-in Linux/root integration test using an isolated API/nginx instance.

Usage: sudo python3 phpfpm_integration.py /absolute/path/fasthost-api [8.4]
Requires nginx, certbot, PHP CLI/FPM + pdo_sqlite + Argon2id and systemd.
Does not replace the production API, frontend, or nginx configuration.
"""
import http.cookiejar
import http.client
import io
import json
import os
from pathlib import Path
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import zipfile


def main():
    if os.geteuid() != 0:
        raise SystemExit("Run as root on the target Linux host")
    binary = str(Path(sys.argv[1]).resolve())
    version = sys.argv[2] if len(sys.argv) > 2 else "8.4"
    root = Path(tempfile.mkdtemp(prefix="fasthost-php-test-"))
    root.chmod(0o755)
    project = None
    pool = None
    process = None
    api = "http://127.0.0.1:18081"
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

    def request(method, path, body=None, headers=None):
        headers = dict(headers or {})
        if isinstance(body, (dict, list)):
            body = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        response = opener.open(urllib.request.Request(api+path, body, headers, method=method), timeout=40)
        return json.load(response)

    try:
        with (root / "api.log").open("w") as log:
            process = subprocess.Popen([
                binary, "--port", "18081", "--data", str(root), "--dsn", str(root/"data/store"),
                "--nginx-conf", str(root/"data/nginx/nginx.conf"),
                "--nginx-http-port", "18080", "--nginx-https-port", "18443",
                "--ssl-provider", "selfsigned", "--php-version", version,
                "--metrics-interval", "2",
            ], cwd=root, stdout=log, stderr=log)
        for attempt in range(100):
            try:
                urllib.request.urlopen(api+"/api/health", timeout=1).close()
                break
            except (OSError, urllib.error.URLError):
                if process.poll() is not None:
                    raise RuntimeError((root/"api.log").read_text())
                time.sleep(0.1)
        else:
            raise RuntimeError("API did not start")
        request("POST", "/api/auth/login", {"username":"admin", "password":"admin"})
        project = request("POST", "/api/projects", {
            "name":"php-integration", "domain":"fasthost-integration.test", "provider":"php-fpm",
        })["project"]
        base = "/api/projects/"+str(project["id"])
        request("POST", base+"/data-directories", {"name":"database", "env":"BROSAYU_DATA_DIR"})
        request("PUT", base+"/data-directories/database/files/config.json", {
            "content":'{"origin":"https://brosayukurit.online"}',
        })
        directory = request("GET", base+"/data-directories")["directories"][0]
        data_path = Path(directory["path"])
        assert not data_path.is_relative_to(Path(root/"users"))
        assert data_path.stat().st_mode & 0o777 == 0o700
        assert data_path.stat().st_uid != 0
        assert (data_path/"config.json").stat().st_uid == data_path.stat().st_uid
        pool = data_path.parent.parent.name
        script = b'''<?php
        $d = getenv('BROSAYU_DATA_DIR');
        $db = new PDO('sqlite:' . $d . '/app.sqlite');
        $db->exec('CREATE TABLE IF NOT EXISTS probe (id INTEGER)');
        $hash = password_hash('test', PASSWORD_ARGON2ID);
        header('Content-Type: application/json');
        echo json_encode(['directory'=>$d, 'origin'=>json_decode(file_get_contents($d.'/config.json'),true)['origin'],
          'argon'=>password_verify('test',$hash), 'sqlite'=>$db->getAttribute(PDO::ATTR_DRIVER_NAME),
          'uid'=>posix_geteuid(), 'outside'=>@file_get_contents('/etc/passwd')===false]);
        '''
        key = request("GET", "/api/users/me/api-key")["api_key"]

        def deploy():
            archive = io.BytesIO()
            with zipfile.ZipFile(archive,"w") as z:
                z.writestr("index.php", script)
            request("POST", "/api/deploy/"+str(project["id"]), archive.getvalue(), {
                "X-Api-Key":key, "Content-Type":"application/zip",
            })

        deploy()
        # Read through nginx -> the real per-project FPM socket, not PHP CLI.
        response = urllib.request.urlopen(urllib.request.Request("http://127.0.0.1:18080/", headers={"Host":project["domain"]}),timeout=30)
        result = json.load(response)
        assert result["directory"] == str(data_path), result
        assert result["origin"] == "https://brosayukurit.online", result
        assert result["argon"] and result["outside"] and result["sqlite"] == "sqlite", result
        assert result["uid"] == data_path.stat().st_uid, result
        deploy()
        assert (data_path/"app.sqlite").exists(), "redeploy removed persistent database"
        assert json.loads((data_path/"config.json").read_text())["origin"] == result["origin"]
        request("POST", base+"/apply-ssl")
        context = ssl.create_default_context(cafile=str(root/"data/ssl/ca.pem"))
        class LoopbackHTTPS(http.client.HTTPSConnection):
            def connect(self):
                self.sock = context.wrap_socket(socket.create_connection(("127.0.0.1",18443),timeout=30),server_hostname=project["domain"])
        connection = LoopbackHTTPS(project["domain"],18443,context=context,timeout=30)
        connection.request("GET","/",headers={"Host":project["domain"]})
        response = connection.getresponse()
        assert response.status == 200
        result = json.loads(response.read())
        connection.close()
        assert result["argon"] and result["sqlite"] == "sqlite"
        redirect = http.client.HTTPConnection("127.0.0.1",18080,timeout=30)
        redirect.request("GET","/",headers={"Host":project["domain"]})
        response = redirect.getresponse()
        assert response.status == 301 and response.getheader("Location") == "https://"+project["domain"]+":18443/"
        response.read()
        redirect.close()
        time.sleep(3)
        metrics = request("GET", base+"/metrics")
        assert metrics["status"] == "ok" and metrics["updated_at"], metrics
        assert metrics["storage"]["data_bytes"] >= (data_path/"app.sqlite").stat().st_size, metrics
        assert request("POST",base+"/metrics/refresh")["status"] == "ok"
        memory = request("GET","/api/admin/stats")["memory"]
        assert memory["available"] and memory["total_bytes"] > memory["available_bytes"], memory
        assert metrics["disk_bytes"] >= (data_path/"app.sqlite").stat().st_size, metrics
        assert metrics["requests_cum"] > 0, metrics
        request("PUT", base+"/enabled", {"enabled":False})
        assert not Path("/etc/php/"+version+"/fpm/pool.d/"+pool+".conf").exists()
        assert (data_path/"app.sqlite").exists()
        request("PUT", base+"/enabled", {"enabled":True})
        assert Path("/etc/php/"+version+"/fpm/pool.d/"+pool+".conf").exists()
        connection = LoopbackHTTPS(project["domain"],18443,context=context,timeout=30)
        connection.request("GET","/",headers={"Host":project["domain"]})
        response = connection.getresponse()
        assert response.status == 200
        assert json.loads(response.read())["sqlite"] == "sqlite"
        connection.close()
        request("DELETE", base)
        project = None
        assert not data_path.exists(), "project deletion left persistent data"
        print("PASS: isolated pool, 0700/owner, environment, config.json, PDO SQLite, Argon2id, open_basedir, ZIP persistence, SSL, metrics, disable/enable, deletion")
    except Exception:
        print((root/"api.log").read_text())
        for error_log in root.rglob("error.log"):
            print(str(error_log), error_log.read_text(errors="replace")[-8000:])
        raise
    finally:
        if project is not None:
            try:
                request("DELETE", "/api/projects/"+str(project["id"]))
            except Exception:
                pass
        if process is not None:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
        nginx_conf = root/"data/nginx/nginx.conf"
        if nginx_conf.exists():
            subprocess.run(["nginx","-c",str(nginx_conf),"-s","quit"],capture_output=True)
        if pool:
            config = Path("/etc/php/"+version+"/fpm/pool.d/"+pool+".conf")
            if config.exists():
                config.unlink()
                subprocess.run(["systemctl","reload","php"+version+"-fpm"],check=True)
            subprocess.run(["userdel",pool],capture_output=True)
        # mkdtemp returns an absolute path inside the system temp directory.
        if root.parent == Path(tempfile.gettempdir()) and root.name.startswith("fasthost-php-test-"):
            shutil.rmtree(root)


if __name__ == "__main__":
    main()
