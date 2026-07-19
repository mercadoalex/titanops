module github.com/mercadoalex/titanops/eval/runner

go 1.22.0

require (
	github.com/mercadoalex/titanops/correlation v0.0.0
	github.com/mercadoalex/titanops/shared/titanops-export v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	github.com/mercadoalex/titanops/correlation => ../../correlation
	github.com/mercadoalex/titanops/shared/titanops-export => ../../shared/titanops-export
)
