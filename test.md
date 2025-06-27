# 🎉 {{ .ProjectName }} {{ .Version }}
**Release Date:** {{ .Date }}

High-performance proxy configuration checker and validator.
# 📦 Installation

## Quick Install (Linux/macOS)

### Download and install latest version
```
curl -sSL "https://github.com/NamiraNet/namira-core/releases/download/{{ .Tag }}/namira-core_{{ .Version }}_$(uname -s)_$(uname -m).tar.gz" | tar -xz
chmod +x namira-core
sudo mv namira-core /usr/local/bin/
```

### Or download binary directly from assets below


## Docker

```
docker run --rm -p 8080:8080 ghcr.io/namiranet/namira-core:{{ .Version }}
```

# 🚀 Quick Start


## Start API server
```
namira-core api --port 8080
```

## Check health
```
curl http://localhost:8080/health
```

## Test configurations
```
curl -X POST http://localhost:8080/scan \
    -H "Content-Type: application/json" \
    -d '{"configs": ["vmess://..."]}'
```

## What's New

---
    
**Checksums**: `checksums.txt` • **Docker**: `ghcr.io/namiranet/namira-core:{{ .Version }}`
    
[📖 Docs](https://github.com/NamiraNet/namira-core#readme) • [🐛 Issues](https://github.com/NamiraNet/namira-core/issues) • [💬 Discussions](https://github.com/NamiraNet/namira-core/discussions)
