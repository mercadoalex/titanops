// Version: v0.1.0
// Tag: shared/titanops-platform/v0.1.0
module github.com/mercadoalex/titanops/shared/titanops-platform

go 1.22.0

require (
	github.com/mercadoalex/titanops/shared/titanops-ai v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-export v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-k8s v0.0.0
)

replace (
	github.com/mercadoalex/titanops/shared/titanops-ai => ../titanops-ai
	github.com/mercadoalex/titanops/shared/titanops-export => ../titanops-export
	github.com/mercadoalex/titanops/shared/titanops-k8s => ../titanops-k8s
)
