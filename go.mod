module github.com/jmaister/taronja-gateway

go 1.24.2

require gopkg.in/yaml.v3 v3.0.1

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.25.0 // indirect
)

require (
	github.com/joho/godotenv v1.5.1
	github.com/lucsky/cuid v1.2.1
	github.com/spf13/cobra v1.9.1
	github.com/stretchr/testify v1.10.0
	golang.org/x/crypto v0.38.0
	golang.org/x/oauth2 v0.30.0
	gorm.io/driver/sqlite v1.5.7
	gorm.io/gorm v1.26.1
)

replace github.com/jmaister/taronja-gateway => ./
