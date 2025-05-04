module github.com/jmaister/taronja-gateway

go 1.24.2

require (
	github.com/go-delve/delve v1.24.2
	golang.org/x/oauth2 v0.29.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cilium/ebpf v0.11.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/arch v0.11.0 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/telemetry v0.0.0-20241106142447-58a1122356f5 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/joho/godotenv v1.5.1
	github.com/stretchr/testify v1.10.0
)

replace github.com/jmaister/taronja-gateway => ./
