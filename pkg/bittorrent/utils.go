// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package bittorrent

import (
	"io"
	"os"
	"path"

	"github.com/pkg/errors"

	"github.com/penglongli/accelerboat/pkg/utils"
)

func (th *TorrentHandler) copySourceToTorrent(sourceFile, digest string) (string, error) {
	torrentFile := path.Join(th.op.StorageConfig.TorrentPath, utils.LayerFileName(digest))
	_ = os.RemoveAll(torrentFile)
	torrentFi, err := os.Create(torrentFile)
	if err != nil {
		return torrentFile, errors.Wrapf(err, "create torrent file '%s' failed", torrentFile)
	}
	defer torrentFi.Close()
	source, err := os.Open(sourceFile)
	if err != nil {
		return torrentFile, errors.Wrapf(err, "open source file '%s' failed", sourceFile)
	}
	defer source.Close()
	if _, err = io.Copy(torrentFi, source); err != nil {
		return torrentFile, errors.Wrapf(err, "copy source file '%s' to '%s' failed", sourceFile, torrentFile)
	}
	return torrentFile, nil
}
