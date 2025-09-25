package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"gh-repo-research-api/internal/database"
	"gh-repo-research-api/internal/github"
)

func main() {
	client := github.NewClient(os.Getenv("GITHUB_TOKEN"))

	// Initialize database connection
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create tables
	if err := db.CreateRepositoriesTable(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	ctx := context.Background()
	currentCursor := ""

	for {
		result, err := client.GetNextRepositories(ctx, currentCursor)
		if err != nil {
			log.Fatal("Failed to search repositories:", err)
		}

		fmt.Printf("Found %d repositories\n", len(result.Repositories))

		for _, repo := range result.Repositories {
			parts := strings.Split(repo.FullName, "/")
			if len(parts) != 2 {
				continue
			}
			owner, name := parts[0], parts[1]

			hasDockerfile, err := client.HasDockerfile(ctx, owner, name)
			if err != nil {
				fmt.Printf("Error checking Dockerfile for %s: %v\n", repo.FullName, err)
				hasDockerfile = false
			}

			// Convert GitHub repo to database repo
			dbRepo := database.Repository{
				URL:            repo.URL,
				NameWithOwner:  repo.FullName,
				StargazerCount: repo.StargazerCount,
				HasDockerfile:  hasDockerfile,
			}

			// Set primary language if available
			if repo.PrimaryLanguage != nil {
				dbRepo.PrimaryLanguage = &repo.PrimaryLanguage.Name
			}

			// Save to database
			if err := db.InsertRepository(dbRepo); err != nil {
				fmt.Printf("Error saving repository %s: %v\n", repo.FullName, err)
				continue
			}

			status := ""
			if hasDockerfile {
				status = " [HAS DOCKERFILE]"
			}
			
			language := "Unknown"
			if repo.PrimaryLanguage != nil {
				language = repo.PrimaryLanguage.Name
			}

			fmt.Printf("- %s (%d stars, %s)%s - SAVED\n", repo.FullName, repo.StargazerCount, language, status)
		}
		
		if !result.PageInfo.HasNextPage {
			break
		}
		
		if result.PageInfo.EndCursor == nil {
			break
		}
		
		currentCursor = *result.PageInfo.EndCursor
	}

	fmt.Println("Collection completed!")
}
