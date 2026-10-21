module hw-6/gateway

go 1.22

require hw-6/ledger v0.0.0

require (
	github.com/stretchr/testify v1.12.1
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace hw-6/ledger => ../ledger
