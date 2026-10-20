package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

var domainRe = regexp.MustCompile(`^(?i)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func ValidateDomain(d string) error {
	if d == "localhost" {
		return nil
	}
	if !domainRe.MatchString(d) {
		return fmt.Errorf("некорректный домен: %s", d)
	}
	if len(d) > 253 {
		return fmt.Errorf("домен слишком длинный")
	}
	return nil
}

func confDir(projectDir string) string { return filepath.Join(projectDir, ".conf") }

func GenerateConfig(domain, projectPath, email string) string {
	root := filepath.ToSlash(projectPath)
	return fmt.Sprintf(`# FastHost: nginx-конфиг для домена %s
# Сгенерировано панелью FastHost. Не редактируйте вручную — правьте из панели.
#
# КАК ПОДКЛЮЧИТЬ К NGINX (тестирование дома):
#   sudo cp %[3]s/.conf/nginx.conf /etc/nginx/sites-available/%[1]s
#   sudo ln -sfn /etc/nginx/sites-available/%[1]s /etc/nginx/sites-enabled/%[1]s
#   sudo nginx -t && sudo systemctl reload nginx
#
# HTTPS (после получения сертификата certbot добавит сюда блок на 443):
#   sudo bash %[3]s/.conf/certbot.sh

server {
    listen 80;
    listen [::]:80;
    server_name %[1]s www.%[1]s;

    root %[2]s;
    index index.html;

    access_log %[3]s/.conf/access.log;
    error_log %[3]s/.conf/error.log;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location ~* \.(css|js|png|jpg|jpeg|gif|svg|ico|webp|woff2?|ttf)$ {
        expires 7d;
        add_header Cache-Control "public, max-age=604800";
    }

    location ~ /\.conf {
        deny all;
        return 404;
    }
}
`, domain, root, root)
}

func WriteFiles(projectDir, domain, email, owner string) error {
	if err := os.MkdirAll(confDir(projectDir), 0o755); err != nil {
		return err
	}
	cfg := GenerateConfig(domain, projectDir, email)
	if err := os.WriteFile(filepath.Join(confDir(projectDir), "nginx.conf"), []byte(cfg), 0o644); err != nil {
		return err
	}

	certbot := fmt.Sprintf(`#!/usr/bin/env bash
# FastHost: скрипт установки SSL-сертификата (Let's Encrypt / certbot)
# Проект: %[1]s | Домен: %[2]s
#
# Запускается из панели (кнопка «Применить SSL») или вручную:
#   sudo bash certbot.sh
#
# Требования: установлены nginx и certbot (sudo apt install certbot python3-certbot-nginx)
set -euo pipefail

DOMAIN="%[2]s"
EMAIL="%[3]s"

echo "[1/4] Проверка конфигурации nginx..."
nginx -t

echo "[2/4] Перезагрузка nginx..."
systemctl reload nginx

echo "[3/4] Получение сертификата Let's Encrypt..."
certbot --nginx -d "$DOMAIN" -d "www.$DOMAIN" --non-interactive --agree-tos -m "$EMAIL" --redirect

echo "[4/4] Готово. Сертификат установлен, автообновление — certbot renew."
`, owner, domain, email)
	if err := os.WriteFile(filepath.Join(confDir(projectDir), "certbot.sh"), []byte(certbot), 0o755); err != nil {
		return err
	}

	install := fmt.Sprintf(`#!/usr/bin/env bash
# FastHost: установка nginx-конфига в систему
# Проект: %[1]s | Домен: %[2]s
# Требует root: sudo bash install.sh
set -euo pipefail

SITE="%[2]s"
CONF="$(dirname "$0")/nginx.conf"

if [ "$EUID" -ne 0 ]; then
  echo "Запустите с sudo: sudo bash install.sh"
  exit 1
fi

cp "$CONF" "/etc/nginx/sites-available/$SITE"
ln -sfn "/etc/nginx/sites-available/$SITE" "/etc/nginx/sites-enabled/$SITE"
nginx -t
systemctl reload nginx

echo "Сайт %[2]s включён. Далее получите SSL: sudo bash certbot.sh (или кнопка «Применить SSL» в панели)."
`, owner, domain)
	if err := os.WriteFile(filepath.Join(confDir(projectDir), "install.sh"), []byte(install), 0o755); err != nil {
		return err
	}
	return nil
}

func ReadConfig(projectDir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(confDir(projectDir), "nginx.conf"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func ApplyCertbot(projectDir, domain, email string, live bool) (string, error) {
	script := filepath.Join(confDir(projectDir), "certbot.sh")
	if !live {
		return "Live-режим выключен (запустите сервер с -live-ssl или ENABLE_LIVE_SSL=1). " +
			"Сгенерированные файлы: " + script + " и " + filepath.Join(confDir(projectDir), "install.sh"), nil
	}
	if _, err := os.Stat(script); err != nil {
		return "", fmt.Errorf("скрипт certbot не найден: %v", err)
	}
	cmd := exec.Command("bash", script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}