module github.com/jmaister/taronja-gateway

go 1.24.2

require gopkg.in/yaml.v3 v3.0.1

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
)

require (
	github.com/joho/godotenv v1.5.1
	github.com/stretchr/testify v1.10.0
)

replace github.com/jmaister/taronja-gateway => ./
