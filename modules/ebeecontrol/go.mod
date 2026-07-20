module github.com/mercadoalex/titanops/modules/ebeecontrol

go 1.22.0

require (
	github.com/google/uuid v1.3.0
	github.com/mercadoalex/titanops/shared/titanops-ai v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-config v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-export v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-k8s v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-platform v0.0.0
	pgregory.net/rapid v1.1.0
)

replace (
	github.com/mercadoalex/titanops/shared/titanops-ai => ../../shared/titanops-ai
	github.com/mercadoalex/titanops/shared/titanops-config => ../../shared/titanops-config
	github.com/mercadoalex/titanops/shared/titanops-export => ../../shared/titanops-export
	github.com/mercadoalex/titanops/shared/titanops-k8s => ../../shared/titanops-k8s
	github.com/mercadoalex/titanops/shared/titanops-platform => ../../shared/titanops-platform
)
