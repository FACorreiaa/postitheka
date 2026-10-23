package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/FACorreiaa/postitheka/internal/analytics"
	"github.com/FACorreiaa/postitheka/internal/db"
	"github.com/FACorreiaa/postitheka/internal/publisher"
	"github.com/FACorreiaa/postitheka/internal/publisher/discord"
)

type Account = db.CreateAccountRow

// ConnectDiscord verifies a webhook with Discord and stores it sealed.
func (s *Service) ConnectDiscord(ctx context.Context, userID uuid.UUID, webhookURL string, displayName *string) (Account, error) {
	conn, err := s.discord.Connect(ctx, webhookURL)
	if errors.Is(err, discord.ErrInvalidWebhook) {
		return Account{}, &BadRequest{Message: "that is not a working Discord webhook URL"}
	}
	if err != nil {
		return Account{}, err
	}
	return s.storeAccount(ctx, userID, publisher.Discord, conn, displayName)
}

// storeAccount is shared by every platform's connect flow. The id is minted
// here, before the insert, because the credentials are sealed to it.
func (s *Service) storeAccount(ctx context.Context, userID uuid.UUID, platform publisher.Platform, conn publisher.Connection, displayName *string) (Account, error) {
	name := conn.DisplayName
	if displayName != nil && strings.TrimSpace(*displayName) != "" {
		name = strings.TrimSpace(*displayName)
	}
	if name == "" {
		name = string(platform)
	}

	id := uuid.New()
	acct, err := s.q.CreateAccount(ctx, db.CreateAccountParams{
		ID:          id,
		UserID:      userID,
		Platform:    string(platform),
		DisplayName: name,
		Credentials: s.box.Seal(conn.Credentials, id[:]),
	})
	if err != nil {
		return Account{}, fmt.Errorf("store account: %w", err)
	}
	s.events.Capture(analytics.Event{
		Name:       analytics.EventAccountConnected,
		DistinctID: userID.String(),
		Props:      analytics.Props("platform", string(platform)),
	})
	return acct, nil
}

func (s *Service) ListAccounts(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	rows, err := s.q.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(rows))
	for i, r := range rows {
		out[i] = Account(r)
	}
	return out, nil
}

// DeleteAccount disconnects an account. Its post targets keep their history
// with a NULL account; any still queued fail with "account disconnected".
func (s *Service) DeleteAccount(ctx context.Context, userID, accountID uuid.UUID) error {
	n, err := s.q.DeleteAccount(ctx, db.DeleteAccountParams{ID: accountID, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
