
* The project is an Application and API Gateway to put in front of your microservices. Helping with routing, authentication, sessions, user management, etc.
* The project is done with Go language.

* For the the gateway, use the Go standard library net/http package to create a simple HTTP server.

* Database acces is done with Gorm, a Go ORM library. Use it to interact with the database.

* The command line actions done from the AI chat, should use Linux shell commands.

* For Go tests, use Testify (https://github.com/stretchr/testify) for the assertions. Use the Go testing package for the tests.

* Use the Makefile for commands:
    * `make test` to run the tests
    * `make build` to build the project
    * `make run` to run the project
