package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PostgresStore keeps contributors, sign-in tokens, and sessions in the
// catalogue database.
type PostgresStore struct{ database *sql.DB }

func NewPostgresStore(database *sql.DB) *PostgresStore { return &PostgresStore{database: database} }

func (store *PostgresStore) Contributor(ctx context.Context, email string) (Contributor, bool, error) {
	return scanContributor(store.database.QueryRowContext(ctx,
		`SELECT email, role, display_name FROM allowed_contributors WHERE email = $1`, email))
}

// ListContributors returns the whole allow-list, newest last.
func (store *PostgresStore) ListContributors(ctx context.Context) ([]Contributor, error) {
	rows, err := store.database.QueryContext(ctx, `SELECT email, role, display_name FROM allowed_contributors ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list contributors: %w", err)
	}
	defer rows.Close()
	contributors := []Contributor{}
	for rows.Next() {
		var contributor Contributor
		if err := rows.Scan(&contributor.Email, &contributor.Role, &contributor.DisplayName); err != nil {
			return nil, fmt.Errorf("scan contributor: %w", err)
		}
		contributors = append(contributors, contributor)
	}
	return contributors, rows.Err()
}

// AddContributor upserts an address so re-adding an existing one can update its
// role or display name.
func (store *PostgresStore) AddContributor(ctx context.Context, email, role, displayName string) (Contributor, error) {
	normalized, valid := NormalizeEmail(email)
	if !valid {
		return Contributor{}, fmt.Errorf("%q is not a usable email address", email)
	}
	if role != "contributor" && role != "owner" {
		return Contributor{}, fmt.Errorf("role must be contributor or owner, got %q", role)
	}
	contributor, stored, err := scanContributor(store.database.QueryRowContext(ctx,
		`INSERT INTO allowed_contributors (email, role, display_name) VALUES ($1, $2, $3)
		 ON CONFLICT (email) DO UPDATE SET role = EXCLUDED.role, display_name = EXCLUDED.display_name
		 RETURNING email, role, display_name`, normalized, role, displayName))
	if err != nil {
		return Contributor{}, err
	}
	if !stored {
		return Contributor{}, errors.New("the contributor was not stored")
	}
	return contributor, nil
}

func (store *PostgresStore) RemoveContributor(ctx context.Context, email string) (bool, error) {
	normalized, valid := NormalizeEmail(email)
	if !valid {
		return false, fmt.Errorf("%q is not a usable email address", email)
	}
	result, err := store.database.ExecContext(ctx, `DELETE FROM allowed_contributors WHERE email = $1`, normalized)
	if err != nil {
		return false, fmt.Errorf("remove contributor: %w", err)
	}
	removed, err := result.RowsAffected()
	return removed > 0, err
}

func (store *PostgresStore) CountRecentLoginTokens(ctx context.Context, email string, since time.Time) (int, error) {
	var count int
	err := store.database.QueryRowContext(ctx,
		`SELECT count(*) FROM admin_login_tokens WHERE email = $1 AND created_at > $2`, email, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count login tokens: %w", err)
	}
	return count, nil
}

// SaveLoginToken stores a hashed link token and prunes this address's spent and
// expired ones, which keeps the table bounded without a background job.
func (store *PostgresStore) SaveLoginToken(ctx context.Context, tokenHash []byte, email string, expiresAt time.Time) error {
	_, err := store.database.ExecContext(ctx, `
		WITH pruned AS (
			DELETE FROM admin_login_tokens WHERE email = $2 AND (expires_at < now() OR consumed_at IS NOT NULL)
		)
		INSERT INTO admin_login_tokens (token_hash, email, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, email, expiresAt)
	if err != nil {
		return fmt.Errorf("save login token: %w", err)
	}
	return nil
}

// ConsumeLoginToken spends a link token exactly once: the update only matches a
// row that is still unspent and unexpired.
func (store *PostgresStore) ConsumeLoginToken(ctx context.Context, tokenHash []byte, now time.Time) (Contributor, error) {
	contributor, found, err := scanContributor(store.database.QueryRowContext(ctx, `
		UPDATE admin_login_tokens AS token SET consumed_at = $2
		FROM allowed_contributors AS contributor
		WHERE token.token_hash = $1 AND token.consumed_at IS NULL AND token.expires_at > $2 AND contributor.email = token.email
		RETURNING contributor.email, contributor.role, contributor.display_name`, tokenHash, now))
	if err != nil {
		return Contributor{}, err
	}
	if !found {
		return Contributor{}, ErrInvalidToken
	}
	_, err = store.database.ExecContext(ctx, `UPDATE allowed_contributors SET last_login_at = $2 WHERE email = $1`, contributor.Email, now)
	if err != nil {
		return Contributor{}, fmt.Errorf("record last login: %w", err)
	}
	return contributor, nil
}

func (store *PostgresStore) CreateSession(ctx context.Context, tokenHash []byte, email string, expiresAt time.Time, userAgent string) error {
	_, err := store.database.ExecContext(ctx, `
		WITH pruned AS (
			DELETE FROM admin_sessions WHERE email = $2 AND expires_at < now()
		)
		INSERT INTO admin_sessions (token_hash, email, expires_at, user_agent) VALUES ($1, $2, $3, $4)`,
		tokenHash, email, expiresAt, userAgent)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionContributor resolves a live session and records that it was used.
func (store *PostgresStore) SessionContributor(ctx context.Context, tokenHash []byte, now time.Time) (Contributor, error) {
	contributor, found, err := scanContributor(store.database.QueryRowContext(ctx, `
		UPDATE admin_sessions AS session SET last_seen_at = $2
		FROM allowed_contributors AS contributor
		WHERE session.token_hash = $1 AND session.expires_at > $2 AND contributor.email = session.email
		RETURNING contributor.email, contributor.role, contributor.display_name`, tokenHash, now))
	if err != nil {
		return Contributor{}, err
	}
	if !found {
		return Contributor{}, ErrNoSession
	}
	return contributor, nil
}

func (store *PostgresStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := store.database.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// PruneAuth removes expired tokens and sessions for every address.
func (store *PostgresStore) PruneAuth(ctx context.Context, now time.Time) error {
	if _, err := store.database.ExecContext(ctx,
		`DELETE FROM admin_login_tokens WHERE expires_at < $1 OR consumed_at IS NOT NULL`, now); err != nil {
		return fmt.Errorf("prune login tokens: %w", err)
	}
	if _, err := store.database.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at < $1`, now); err != nil {
		return fmt.Errorf("prune sessions: %w", err)
	}
	return nil
}

func scanContributor(row *sql.Row) (Contributor, bool, error) {
	var contributor Contributor
	if err := row.Scan(&contributor.Email, &contributor.Role, &contributor.DisplayName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Contributor{}, false, nil
		}
		return Contributor{}, false, fmt.Errorf("scan contributor: %w", err)
	}
	return contributor, true, nil
}
