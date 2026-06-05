module github.com/mercadoalex/titanops/modules/ollinai

go 1.22.0

require (
	github.com/google/uuid v1.6.0
	github.com/mercadoalex/titanops/shared/titanops-config v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-export v0.0.0
)

require (
	github.com/kr/text v0.2.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/mercadoalex/titanops/shared/titanops-config => ../../shared/titanops-config
	github.com/mercadoalex/titanops/shared/titanops-export => ../../shared/titanops-export
)
