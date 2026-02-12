// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

// Package cleaner implements automatic cleanup of layer storage directories
// based on CleanConfig: retain days and disk usage threshold with LRU eviction.
package cleaner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/recorder"
)

type ImageCleaner interface {
	Init() error
}

type imageCleaner struct {
	op       *options.AccelerBoatOption
	cronExpr string
	cronObj  *cron.Cron
}

func NewImageCleaner(op *options.AccelerBoatOption) ImageCleaner {
	return &imageCleaner{
		op:       op,
		cronExpr: op.CleanConfig.Cron,
	}
}

func (c *imageCleaner) Init() error {
	if c.cronExpr == "" {
		return nil
	}
	c.cronObj = cron.New()
	_, err := c.cronObj.AddFunc(c.cronExpr, func() {
		if err := c.runClean(context.Background()); err != nil {
			logger.Errorf("[clean] failed clean: %s", err.Error())
		}
	})
	if err != nil {
		return errors.Wrapf(err, "init cleaner failed")
	}
	c.cronObj.Start()
	return nil
}

func (c *imageCleaner) runClean(ctx context.Context) error {
	cfg := &c.op.CleanConfig
	storage := &c.op.StorageConfig
	dirs := []struct {
		label string
		dir   string
	}{
		{"torrent", storage.TorrentPath},
		{"transfer", storage.TransferPath},
		{"smallfile", storage.SmallFilePath},
		{"oci", storage.OCIPath},
	}
	totalGB := c.totalDiskUsed(dirs)
	// only check disk usage exceeds the threshold
	if cfg.RetainDays == 0 {
		exceeds := totalGB > float64(cfg.Threshold)
		if !exceeds {
			logger.InfoContextf(ctx, "[clean] disk used: %.2fGB, not exceed threshold: %dGB",
				totalGB, cfg.Threshold)
			return nil
		}
	}
	// logger.InfoContextf(ctx, "[clean] disk used: %.2fGB, threshold: %dGB", totalGB, cfg.Threshold)
	digestLastUsed := buildDigestLastUsedFromEvents(c.op.StorageConfig.EventFile, c.op.CleanConfig.RetainDays)
	candidates, err := collectLayerFilesWithLRU(dirs, digestLastUsed)
	if err != nil {
		return errors.Wrap(err, "collect layer files with lru failed")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastUsed.Before(candidates[j].lastUsed)
	})
	var freedGB float64
	targetGB := float64(cfg.Threshold)
	for _, c := range candidates {
		if totalGB-freedGB <= targetGB {
			break
		}
		if err = os.Remove(c.path); err != nil {
			if !os.IsNotExist(err) {
				logger.ErrorContextf(ctx, "[clean] remove %s failed: %s", c.path, err.Error())
			}
			continue
		}
		freedGB += c.sizeGB
		logger.InfoContextf(ctx, "[clean] removed layer file %s (%.4g GB)", c.path, c.sizeGB)
	}
	if freedGB > 0 {
		logger.InfoContextf(ctx, "[clean] freed %.4g GB (total was %.4g GB, threshold %d GB)",
			freedGB, totalGB, cfg.Threshold)
	}
	return nil
}

func (c *imageCleaner) totalDiskUsed(dirs []struct {
	label string
	dir   string
}) float64 {
	const bytesPerGB = 1e9
	var total int64
	for _, d := range dirs {
		size, err := metrics.DirSizeBytes(d.dir)
		if err != nil {
			continue
		}
		total += size
	}
	return float64(total) / bytesPerGB
}

const (
	// listEventsLimit is the max events to read from event file for LRU ordering.
	listEventsLimit = 500000
)

// buildDigestLastUsedFromEvents reads events (from recorder when event file is set) and returns
// digest -> latest timestamp for blob-related events that carry a digest in Details.
func buildDigestLastUsedFromEvents(eventFile string, retainDays int64) map[string]time.Time {
	out := make(map[string]time.Time)
	var events []recorder.Event
	if eventFile != "" {
		var startTime *time.Time
		if retainDays != 0 {
			t := time.Now().Add(-time.Duration(retainDays) * 24 * time.Hour)
			startTime = &t
		}
		events = recorder.Global.List(listEventsLimit, []string{
			string(recorder.EventServeBlobFromLocal),
			string(recorder.EventTypeGetBlobFromMaster),
			string(recorder.EventTypeDownloadBlobByTCP),
			string(recorder.EventTypeDownloadBlobByTorrent),
		}, startTime)
	}
	blobTypes := map[recorder.EventType]bool{
		recorder.EventServeBlobFromLocal:        true,
		recorder.EventTypeGetBlobFromMaster:     true,
		recorder.EventTypeDownloadBlobByTCP:     true,
		recorder.EventTypeDownloadBlobByTorrent: true,
	}
	for _, e := range events {
		if !blobTypes[e.Type] {
			continue
		}
		digest := digestFromDetails(e.Details)
		if digest == "" {
			continue
		}
		norm := normalizeDigest(digest)
		if e.Timestamp.After(out[norm]) {
			out[norm] = e.Timestamp
		}
	}
	return out
}

func digestFromDetails(details map[string]interface{}) string {
	if details == nil {
		return ""
	}
	v, ok := details["digest"]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func normalizeDigest(d string) string {
	d = strings.TrimSpace(d)
	if strings.HasPrefix(d, "sha256:") {
		return d
	}
	return "sha256:" + d
}

type layerFile struct {
	path     string
	sizeGB   float64
	lastUsed time.Time
}

func collectLayerFilesWithLRU(dirs []struct{ label, dir string }, digestLastUsed map[string]time.Time) (
	[]*layerFile, error) {
	const bytesPerGB = 1e9
	var out []*layerFile
	for _, d := range dirs {
		err := filepath.WalkDir(d.dir, func(entryPath string, de fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if de.IsDir() || !strings.HasSuffix(de.Name(), ".tar.gzip") {
				return nil
			}
			info, err := de.Info()
			if err != nil {
				return nil
			}
			digest := digestFromLayerFileName(de.Name(), d.label == "oci")
			lastUsed := digestLastUsed[normalizeDigest(digest)]
			out = append(out, &layerFile{
				path:     entryPath,
				sizeGB:   float64(info.Size()) / bytesPerGB,
				lastUsed: lastUsed,
			})
			return nil
		})
		if err != nil {
			return nil, errors.Wrapf(err, "walk %s", d.dir)
		}
	}
	return out, nil
}

// digestFromLayerFileName derives digest from a layer file base name.
// OCI layer files are "hex.tar.gzip" (no sha256: prefix); legacy or other dirs may use "sha256:hex.tar.gzip".
func digestFromLayerFileName(base string, isOCI bool) string {
	name := strings.TrimSuffix(base, ".tar.gzip")
	if name == base {
		return ""
	}
	return "sha256:" + strings.TrimPrefix(name, "sha256:")
}
