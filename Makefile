PLUGIN ?= qoder
OUT ?= dist

.PHONY: test vet build clean

test:
	go test ./... -shuffle=on -count=1

vet:
	go vet ./...

build:
	mkdir -p $(OUT)
	CGO_ENABLED=1 go build -buildmode=c-shared -trimpath -ldflags='-s -w' -o $(OUT)/$(PLUGIN).so .

clean:
	rm -rf $(OUT)
