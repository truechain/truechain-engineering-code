// Copyright 2018 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

package signer

import (
	"context"
	"github.com/truechain/truechain-engineering-code/accounts"
	"github.com/truechain/truechain-engineering-code/cmd/truekey/types"
	"github.com/truechain/truechain-engineering-code/log"
)

type ServerAuditLogger struct {
	log log.Logger
	api types.ServerAPI
}

func (l *ServerAuditLogger) RegisterAdmin(ctx context.Context, passphrase string) error {
	l.log.Info("RegisterAdmin", "type", "request", "metadata", MetadataFromContext(ctx).String(), "passphrase", passphrase)
	e := l.api.RegisterAdmin(ctx, passphrase)
	l.log.Info("RegisterAdmin", "type", "response", "error", e)

	return e
}

func (l *ServerAuditLogger) DeriveAccounts(ctx context.Context, count uint64, passphrase string) ([]accounts.Account, error) {
	l.log.Info("DeriveAccounts", "type", "request", "metadata", MetadataFromContext(ctx).String(),
		"count", count,
		"passphrase", passphrase)

	res, e := l.api.DeriveAccounts(ctx, count, passphrase)
	if res != nil {
		l.log.Info("DeriveAccounts", "type", "response", "data", res, "error", e)
	} else {
		l.log.Info("DeriveAccounts", "type", "response", "data", res, "error", e)
	}
	return res, e
}

func (l *ServerAuditLogger) ChildAddress(ctx context.Context, passphrase string) ([]types.ChildAccount, error) {
	l.log.Info("ChildAddress", "type", "request", "metadata", MetadataFromContext(ctx).String(),
		"passphrase", passphrase)
	data, err := l.api.ChildAddress(ctx, passphrase)
	l.log.Info("ChildAddress", "type", "response", "data", data, "error", err)
	return data, err

}

func (l *ServerAuditLogger) UpdateAccount(ctx context.Context, passphrase string, id uint64, content types.AccountState) error {
	l.log.Info("UpdateAccount", "type", "request", "metadata", MetadataFromContext(ctx).String(),
		"passphrase", passphrase,
		"id", id,
		"content", content,
	)
	err := l.api.UpdateAccount(ctx, passphrase, id, content)
	l.log.Info("UpdateAccount", "type", "response", "error", err)
	return err

}

func (l *ServerAuditLogger) ChangeAdmin(ctx context.Context, passphrase string, newPassphrase string) error {
	l.log.Info("ChangeAdmin", "type", "request", "metadata", MetadataFromContext(ctx).String(),
		"passphrase", passphrase,
		"newPassphrase", newPassphrase,
	)
	err := l.api.ChangeAdmin(ctx, passphrase, newPassphrase)
	l.log.Info("ChangeAdmin", "type", "response", "error", err)
	return err

}

func NewServerAuditLogger(path string, api types.ServerAPI) (*ServerAuditLogger, error) {
	l := log.New("api", "signer")
	handler, err := log.FileHandler(path, log.LogfmtFormat())
	if err != nil {
		return nil, err
	}
	l.SetHandler(handler)
	l.Info("Configured", "server audit log", path)
	return &ServerAuditLogger{l, api}, nil
}
