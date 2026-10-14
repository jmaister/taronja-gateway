package cli

import (
	"log"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func newAddUserCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adduser [username] [email] [password]",
		Short: "Create a new user in the DB",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			addUser(args[0], args[1], args[2])
		},
	}
}

func addUser(username, email, password string) {
	if err := godotenv.Load(); dotEnvLoadIsFatal(err) {
		log.Printf("Warning: Failed to load .env file: %v", err)
	}

	appDependencies := deps.NewProduction()

	newUser := &db.User{
		Username:       username,
		Email:          email,
		Password:       password,
		EmailConfirmed: false,
	}

	if err := appDependencies.UserRepo.CreateUser(newUser); err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	log.Printf("User '%s' created successfully with email '%s'.", username, email)
}
