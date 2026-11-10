PI      ?= raspi
BIN     := bin/dashboard-arm64
CONF    := $(HOME)/.config/home-dashboard
UNIT    := home-dashboard.service

.PHONY: test demo build deploy secrets logs

test:
	go vet ./...
	go test ./...

demo:
	go run ./cmd/dashboard serve --demo --addr 127.0.0.1:8090

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/dashboard

# Copia el binario y la unidad systemd de usuario a la Raspberry y reinicia.
deploy: test build
	ssh $(PI) 'mkdir -p ~/.local/bin ~/.config/systemd/user ~/.config/home-dashboard ~/.local/state/home-dashboard'
	scp -q $(BIN) $(PI):.local/bin/dashboard.new
	scp -q deploy/$(UNIT) $(PI):.config/systemd/user/$(UNIT)
	ssh $(PI) 'mv ~/.local/bin/dashboard.new ~/.local/bin/dashboard && systemctl --user daemon-reload && systemctl --user enable -q $(UNIT) && systemctl --user restart $(UNIT) && sleep 1 && systemctl --user --no-pager -n 5 status $(UNIT)'

# Tras `go run ./cmd/dashboard auth`: sube credenciales y token, y sale del modo demo.
secrets:
	scp -q -p $(CONF)/credentials.json $(CONF)/token.json $(PI):.config/home-dashboard/
	ssh $(PI) 'chmod 600 ~/.config/home-dashboard/*.json && rm -f ~/.config/home-dashboard/env && systemctl --user restart $(UNIT)'

logs:
	ssh $(PI) 'tail -n 50 ~/.local/state/home-dashboard/dashboard.log'

# Copia la app y el scriptlet al Kindle por SSH (Jobman, modo USB).
KINDLE ?= root@192.168.15.244
.PHONY: kindle
kindle:
	ssh $(KINDLE) 'mkdir -p /mnt/us/dashboard/waf'
	scp -q -O deploy/kindle/waf/config.xml deploy/kindle/waf/index.html $(KINDLE):/mnt/us/dashboard/waf/
	scp -q -O deploy/kindle/dashboard.sh $(KINDLE):/mnt/us/documents/dashboard.sh
