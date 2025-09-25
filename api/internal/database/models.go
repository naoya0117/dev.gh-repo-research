package database

import (
	"database/sql"
	"time"
)

type Repository struct {
	ID              int          `json:"id"`
	URL             string       `json:"url"`
	NameWithOwner   string       `json:"nameWithOwner"`
	StargazerCount  int          `json:"stargazerCount"`
	PrimaryLanguage *string      `json:"primaryLanguage"`
	HasDockerfile   bool         `json:"hasDockerfile"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

func (db *DB) CreateRepositoriesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS repositories (
			id SERIAL PRIMARY KEY,
			url VARCHAR(255) UNIQUE NOT NULL,
			name_with_owner VARCHAR(255) NOT NULL,
			stargazer_count INTEGER NOT NULL,
			primary_language VARCHAR(100),
			has_dockerfile BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := db.Exec(query)
	return err
}

func (db *DB) InsertRepository(repo Repository) error {
	query := `
		INSERT INTO repositories (url, name_with_owner, stargazer_count, primary_language, has_dockerfile)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (url) DO UPDATE SET
			name_with_owner = EXCLUDED.name_with_owner,
			stargazer_count = EXCLUDED.stargazer_count,
			primary_language = EXCLUDED.primary_language,
			has_dockerfile = EXCLUDED.has_dockerfile,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := db.Exec(query, repo.URL, repo.NameWithOwner, repo.StargazerCount, repo.PrimaryLanguage, repo.HasDockerfile)
	return err
}

func (db *DB) GetRepositories(limit, offset int) ([]Repository, error) {
	query := `
		SELECT id, url, name_with_owner, stargazer_count, primary_language, has_dockerfile, created_at, updated_at
		FROM repositories
		ORDER BY stargazer_count DESC
		LIMIT $1 OFFSET $2
	`
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repositories []Repository
	for rows.Next() {
		var repo Repository
		var primaryLanguage sql.NullString
		
		err := rows.Scan(
			&repo.ID,
			&repo.URL,
			&repo.NameWithOwner,
			&repo.StargazerCount,
			&primaryLanguage,
			&repo.HasDockerfile,
			&repo.CreatedAt,
			&repo.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		if primaryLanguage.Valid {
			repo.PrimaryLanguage = &primaryLanguage.String
		}
		
		repositories = append(repositories, repo)
	}

	return repositories, rows.Err()
}