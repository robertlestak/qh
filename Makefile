bin: bin/qh_darwin_amd64 bin/qh_linux_amd64 bin/qh_windows_amd64.exe

bin/qh_darwin_amd64:
	@mkdir -p bin
	@echo "Compiling qh..."
	GOOS=darwin GOARCH=amd64 go build -o $@ ./cmd/qh/*.go

bin/qh_linux_amd64:
	@mkdir -p bin
	@echo "Compiling qh..."
	GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/qh/*.go

bin/qh_windows_amd64.exe:
	@mkdir -p bin
	@echo "Compiling qh..."
	GOOS=windows GOARCH=amd64 go build -o $@ ./cmd/qh/*.go