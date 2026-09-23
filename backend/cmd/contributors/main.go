// Command contributors manages the allow-list of email addresses that may sign
// in to the catalogue admin app.
//
//	contributors list
//	contributors add <email> [contributor|owner]
//	contributors remove <email>
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fauzanebd/wheretowfc/backend/internal/auth"
	"github.com/fauzanebd/wheretowfc/backend/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	usage := "usage: contributors list | add <email> [contributor|owner] | remove <email>"
	if len(os.Args) < 2 {
		log.Fatal(usage)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	database, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()

	store := auth.NewPostgresStore(database)
	ctx := context.Background()
	switch command := strings.ToLower(os.Args[1]); command {
	case "list":
		contributors, err := store.ListContributors(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if len(contributors) == 0 {
			fmt.Println("no contributors yet — add one with: contributors add you@example.com owner")
			return
		}
		for _, contributor := range contributors {
			fmt.Printf("%-40s %s\n", contributor.Email, contributor.Role)
		}
	case "add":
		if len(os.Args) < 3 {
			log.Fatal(usage)
		}
		role := "contributor"
		if len(os.Args) > 3 {
			role = strings.ToLower(os.Args[3])
		}
		contributor, err := store.AddContributor(ctx, os.Args[2], role, "")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("allowed: %s (%s)\n", contributor.Email, contributor.Role)
	case "remove":
		if len(os.Args) < 3 {
			log.Fatal(usage)
		}
		removed, err := store.RemoveContributor(ctx, os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		if !removed {
			log.Fatalf("%s was not on the allow-list", os.Args[2])
		}
		fmt.Printf("removed %s and any sessions it held\n", strings.ToLower(strings.TrimSpace(os.Args[2])))
	default:
		log.Fatal(usage)
	}
}
