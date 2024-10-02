// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"

	"github.com/gfanton/gnopls/internal/event"
	"github.com/gfanton/gnopls/internal/file"
	"github.com/gfanton/gnopls/internal/golang"
	"github.com/gfanton/gnopls/internal/label"
	"github.com/gfanton/gnopls/internal/protocol"
	"github.com/gfanton/gnopls/internal/telemetry"
)

func (s *server) Implementation(ctx context.Context, params *protocol.ImplementationParams) (_ []protocol.Location, rerr error) {
	recordLatency := telemetry.StartLatencyTimer("implementation")
	defer func() {
		recordLatency(ctx, rerr)
	}()

	ctx, done := event.Start(ctx, "lsp.Server.implementation", label.URI.Of(params.TextDocument.URI))
	defer done()

	fh, snapshot, release, err := s.fileOf(ctx, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	defer release()
	if snapshot.FileKind(fh) != file.Gno {
		return nil, nil // empty result
	}
	return golang.Implementation(ctx, snapshot, fh, params.Position)
}
