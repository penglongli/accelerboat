// Copyright 2025 The AccelerBoat Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package customapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/cmd/accelerboat/options/leaderselector"
)

type statsJSON struct {
	ContainerdEnabled bool                `json:"containerdEnabled"`
	Torrent           torrentStatsJSON    `json:"torrent"`
	Master            string              `json:"master"`
	HTTPProxy         string              `json:"httpProxy"`
	Upstreams         []upstreamEntryJSON `json:"upstreams"`
	Storage           []storageEntryJSON  `json:"storage"`
	Cleanup           cleanStatsJSON      `json:"cleanup"`
	Transfer          []transferEntryJSON `json:"transfer"`
	ErrorsTotal       int64               `json:"errorsTotal"`
}

type torrentStatsJSON struct {
	Enabled       bool   `json:"enabled"`
	Threshold     int64  `json:"threshold"`
	UploadLimit   int64  `json:"uploadLimit"`
	DownloadLimit int64  `json:"downloadLimit"`
	Announce      string `json:"announce"`
	ManagedCount  int    `json:"managedCount"`
}

type storageEntryJSON struct {
	Path    string  `json:"path"`
	Label   string  `json:"label"`
	UsageGB float64 `json:"usageGB"`
}

type cleanStatsJSON struct {
	Enabled    bool  `json:"enabled"`
	Threshold  int64 `json:"threshold"`
	RetainDays int64 `json:"retainDays"`
}

type transferEntryJSON struct {
	Operation string  `json:"operation"`
	SizeGB    float64 `json:"sizeGB"`
}

type upstreamEntryJSON struct {
	ProxyHost    string `json:"proxyHost"`
	OriginalHost string `json:"originalHost"`
	Enabled      bool   `json:"enabled"`
}

// storageLabelOrder matches options.StorageConfig fields for ordered output of directories.
var storageLabelOrder = []struct {
	Label string
	Path  func(op *options.AccelerBoatOption) string
}{
	{"transfer", func(op *options.AccelerBoatOption) string { return op.StorageConfig.TransferPath }},
	{"download", func(op *options.AccelerBoatOption) string { return op.StorageConfig.DownloadPath }},
	{"smallfile", func(op *options.AccelerBoatOption) string { return op.StorageConfig.SmallFilePath }},
	{"torrent", func(op *options.AccelerBoatOption) string { return op.StorageConfig.TorrentPath }},
	{"oci", func(op *options.AccelerBoatOption) string { return op.StorageConfig.OCIPath }},
}

// Stats returns runtime stats (storage, transfer, errors, torrent, upstreams) as JSON or formatted text
// (see HTTPWrapperWithOutput).
func (h *CustomHandler) Stats(c *gin.Context) (interface{}, string, error) {
	op := h.op
	tc := op.TorrentConfig
	sm, _ := getStatsMetrics()
	storage := make([]storageEntryJSON, 0, len(storageLabelOrder))
	for _, e := range storageLabelOrder {
		path := e.Path(op)
		usage := sm.DiskUsage[e.Label]
		storage = append(storage, storageEntryJSON{Path: path, Label: e.Label, UsageGB: usage})
	}
	cleanup := cleanStatsJSON{
		Enabled:    op.CleanConfig.Cron != "",
		Threshold:  op.CleanConfig.Threshold,
		RetainDays: op.CleanConfig.RetainDays,
	}
	transfer := make([]transferEntryJSON, 0, len(sm.TransferSize))
	for opName, gb := range sm.TransferSize {
		transfer = append(transfer, transferEntryJSON{Operation: opName, SizeGB: gb})
	}
	sortTransferEntries(transfer)
	js := statsJSON{
		ContainerdEnabled: op.EnableContainerd,
		Torrent: torrentStatsJSON{
			Enabled:       tc.Enable,
			Threshold:     tc.Threshold,
			UploadLimit:   tc.UploadLimit,
			DownloadLimit: tc.DownloadLimit,
			Announce:      tc.Announce,
			ManagedCount:  sm.TorrentActiveCount,
		},
		Master:      leaderselector.CurrentMaster(),
		HTTPProxy:   op.ExternalConfig.HTTPProxy,
		Upstreams:   buildUpstreamsList(op),
		Storage:     storage,
		Cleanup:     cleanup,
		Transfer:    transfer,
		ErrorsTotal: sm.ErrorsTotal,
	}
	text := formatStats(js)
	return js, text, nil
}

func sortTransferEntries(entries []transferEntryJSON) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Operation < entries[j].Operation })
}

func buildUpstreamsList(op *options.AccelerBoatOption) []upstreamEntryJSON {
	list := make([]upstreamEntryJSON, 0, 1+len(op.ExternalConfig.RegistryMappings))
	dh := op.ExternalConfig.DockerHubRegistry
	proxyHost := dh.ProxyHost
	if proxyHost == "" {
		proxyHost = "docker.io"
	}
	list = append(list, upstreamEntryJSON{
		ProxyHost:    proxyHost,
		OriginalHost: dh.OriginalHost,
		Enabled:      dh.Enable,
	})
	for _, m := range op.ExternalConfig.RegistryMappings {
		list = append(list, upstreamEntryJSON{
			ProxyHost:    m.ProxyHost,
			OriginalHost: m.OriginalHost,
			Enabled:      m.Enable,
		})
	}
	return list
}

func formatStats(js statsJSON) string {
	var b strings.Builder
	b.WriteString("=== AccelerBoat Stats ===\n\n")
	b.WriteString(fmt.Sprintf("Containerd:    %s\n", formatBool(js.ContainerdEnabled)))
	b.WriteString(fmt.Sprintf("Torrent:       %s\n", formatBool(js.Torrent.Enabled)))
	if js.Torrent.Enabled {
		b.WriteString(fmt.Sprintf("  Threshold:     %d (MB)\n", js.Torrent.Threshold))
		b.WriteString(fmt.Sprintf("  UploadLimit:   %d (0=unlimited)\n", js.Torrent.UploadLimit))
		b.WriteString(fmt.Sprintf("  DownloadLimit: %d (0=unlimited)\n", js.Torrent.DownloadLimit))
		b.WriteString(fmt.Sprintf("  Announce:      %s\n", js.Torrent.Announce))
		b.WriteString(fmt.Sprintf("  ManagedCount:  %d\n", js.Torrent.ManagedCount))
	}
	b.WriteString(fmt.Sprintf("Master:        %s\n", js.Master))
	b.WriteString(fmt.Sprintf("HTTPProxy:     %s\n", orEmpty(js.HTTPProxy)))
	b.WriteString("\nStorage (disk usage):\n")
	for _, s := range js.Storage {
		b.WriteString(fmt.Sprintf("  [%s] %s  =>  %.4g GB\n", s.Label, s.Path, s.UsageGB))
	}
	b.WriteString("\nCleanup:\n")
	b.WriteString(fmt.Sprintf("  Enabled:    %s\n", formatBool(js.Cleanup.Enabled)))
	b.WriteString(fmt.Sprintf("  Threshold:  %d GB\n", js.Cleanup.Threshold))
	b.WriteString(fmt.Sprintf("  RetainDays: %d\n", js.Cleanup.RetainDays))
	b.WriteString("\nTransfer (cumulative):\n")
	for _, t := range js.Transfer {
		b.WriteString(fmt.Sprintf("  %s  =>  %.4g GB\n", t.Operation, t.SizeGB))
	}
	b.WriteString(fmt.Sprintf("\nErrorsTotal:  %d\n", js.ErrorsTotal))
	b.WriteString("\nUpstreams:\n")
	for _, u := range js.Upstreams {
		b.WriteString(fmt.Sprintf("  - %s -> %s  [Enabled: %s]\n", u.ProxyHost, u.OriginalHost, formatBool(u.Enabled)))
	}
	return b.String()
}

func formatBool(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func orEmpty(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}
