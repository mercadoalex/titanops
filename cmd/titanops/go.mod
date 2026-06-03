// Version: v0.1.0
// Tag: cmd/titanops/v0.1.0
module github.com/mercadoalex/titanops/cmd/titanops

go 1.22.0

require (
	github.com/mercadoalex/titanops/correlation v0.0.0
	github.com/mercadoalex/titanops/gateway v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-ai v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-export v0.0.0
)

replace (
	github.com/mercadoalex/titanops/correlation => ../../correlation
	github.com/mercadoalex/titanops/gateway => ../../gateway
	github.com/mercadoalex/titanops/shared/titanops-ai => ../../shared/titanops-ai
	github.com/mercadoalex/titanops/shared/titanops-export => ../../shared/titanops-export
	github.com/mercadoalex/titanops/shared/titanops-k8s => ../../shared/titanops-k8s
	github.com/mercadoalex/titanops/shared/titanops-config => ../../shared/titanops-config
)
