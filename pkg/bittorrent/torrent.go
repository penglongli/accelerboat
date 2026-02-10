/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bittorrent

import (
	"bytes"
	"context"
	"encoding/base64"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"

	"github.com/penglongli/accelerboat/cmd/accelerboat/options"
	"github.com/penglongli/accelerboat/pkg/logger"
	"github.com/penglongli/accelerboat/pkg/metrics"
	"github.com/penglongli/accelerboat/pkg/store"
	"github.com/penglongli/accelerboat/pkg/utils"
	"github.com/penglongli/accelerboat/pkg/utils/formatutils"
	"github.com/penglongli/accelerboat/pkg/utils/lock"
)

// TorrentHandler defines the torrent handler
type TorrentHandler struct {
	sync.Mutex
	torrentLock lock.Interface
	op          *options.AccelerBoatOption

	client       *torrent.Client
	cacheStore   store.CacheStore
	torrentCache *sync.Map

	semaphore chan struct{}
}

// NewTorrentHandler create the torrent handler instance
func NewTorrentHandler() *TorrentHandler {
	return &TorrentHandler{
		op:           options.GlobalOptions(),
		cacheStore:   store.GlobalRedisStore(),
		torrentLock:  lock.NewLocalLock(),
		torrentCache: &sync.Map{},
		semaphore:    make(chan struct{}, 10),
	}
}

// Init the torrent handler
func (th *TorrentHandler) Init() error {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = th.op.StorageConfig.TorrentPath
	clientConfig.Seed = true
	clientConfig.ListenPort = int(th.op.TorrentPort)
	clientConfig.DisableUTP = true
	clientConfig.MaxUnverifiedBytes = 4096 << 20
	clientConfig.PieceHashersPerTorrent = 8
	clientConfig.NoDHT = true
	clientConfig.DisablePEX = false
	clientConfig.EstablishedConnsPerTorrent = 200
	clientConfig.HalfOpenConnsPerTorrent = 100
	clientConfig.TotalHalfOpenConns = 500
	clientConfig.TorrentPeersHighWater = 2000
	clientConfig.TorrentPeersLowWater = 200
	clientConfig.MaxAllocPeerRequestDataPerConn = 4 << 20
	clientConfig.DialRateLimiter = rate.NewLimiter(100, 200)
	clientConfig.DisableAcceptRateLimiting = true
	clientConfig.AcceptPeerConnections = true
	clientConfig.DefaultStorage = storage.NewMMap(th.op.StorageConfig.TorrentPath)
	if th.op.TorrentConfig.UploadLimit > 0 {
		clientConfig.UploadRateLimiter = rate.NewLimiter(rate.Limit(th.op.TorrentConfig.UploadLimit*options.MB),
			2*int(th.op.TorrentConfig.UploadLimit*options.MB))
	}
	if th.op.TorrentConfig.DownloadLimit > 0 {
		clientConfig.DownloadRateLimiter = rate.NewLimiter(rate.Limit(th.op.TorrentConfig.DownloadLimit*options.MB),
			2*int(th.op.TorrentConfig.DownloadLimit*options.MB))
	}
	tc, err := torrent.NewClient(clientConfig)
	if err != nil {
		return errors.Wrapf(err, "create torrent client failed")
	}
	th.client = tc
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			metrics.TorrentActiveCount.Set(float64(len(th.client.Torrents())))
		}
	}()
	return nil
}

func (th *TorrentHandler) GetClient() *torrent.Client {
	return th.client
}

func (th *TorrentHandler) getLayerFiles(path string) ([]string, error) {
	layerFiles := make([]string, 0)
	if err := filepath.Walk(path, func(fp string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(fp, ".tar.gzip") {
			layerFiles = append(layerFiles, fp)
		}
		return nil
	}); err != nil {
		return nil, errors.Wrapf(err, "read path '%s' failed", th.op.StorageConfig.TorrentPath)
	}
	return layerFiles, nil
}

func (th *TorrentHandler) GenerateTorrent(ctx context.Context, digest, sourceFile string) (string, error) {
	start := time.Now()
	torrentBase64, err := th.handleGenerateTorrent(ctx, digest, sourceFile)
	metrics.TorrentOperationDuration.WithLabelValues("generate").Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.TorrentOperationsTotal.WithLabelValues("generate", "error").Inc()
	} else {
		metrics.TorrentOperationsTotal.WithLabelValues("generate", "success").Inc()
	}
	return torrentBase64, err
}

// GenerateTorrent generate the file to torrent
func (th *TorrentHandler) handleGenerateTorrent(ctx context.Context, digest, sourceFile string) (string, error) {
	th.torrentLock.Lock(ctx, digest)
	defer th.torrentLock.UnLock(ctx, digest)
	to, torrentBase64 := th.CheckTorrentLocalExist(ctx, digest)
	if to != nil {
		logger.InfoContextf(ctx, "torrent already exist, return diectly")
		return torrentBase64, nil
	}

	// copy source-file to torrent path
	torrentFile := path.Join(th.op.StorageConfig.TorrentPath, utils.LayerFileName(digest))
	if err := utils.CopyFile(sourceFile, torrentFile); err != nil {
		return "", err
	}
	var serveTo *torrent.Torrent
	var err error
	generateRetry := 3
	for i := 0; i < generateRetry; i++ {
		serveTo, err = th.generateServeTorrent(ctx, digest, torrentFile)
		if err != nil {
			return "", errors.Wrapf(err, "generate serve torrent failed")
		}
		v, _ := th.CheckTorrentLocalExist(ctx, digest)
		if v != nil {
			logger.InfoContextf(ctx, "check torrent exist in db")
			break
		}
		if i == generateRetry-1 {
			return "", errors.Errorf("generate torrent failed because no torrent in db")
		}
		logger.WarnContextf(ctx, "generate torrent succes but no torrent in db, should generate again(i=%d)", i)
	}
	if serveTo == nil {
		return "", errors.Errorf("generate torrent is nil")
	}

	serveMi := serveTo.Metainfo()
	var buffer bytes.Buffer
	if err = serveMi.Write(&buffer); err != nil {
		return "", errors.Wrapf(err, "get torrent bytes failed")
	}
	torrentBase64 = base64.StdEncoding.EncodeToString(buffer.Bytes())
	logger.InfoContextf(ctx, "generate serve torrent success")
	return torrentBase64, nil
}

func (th *TorrentHandler) generateServeTorrent(ctx context.Context, digest, layerFile string) (*torrent.Torrent, error) {
	fi, err := os.Stat(layerFile)
	if err != nil {
		return nil, err
	}
	pieceLength := metainfo.ChoosePieceLength(fi.Size())
	info := metainfo.Info{
		PieceLength: pieceLength,
	}
	if err = info.BuildFromFilePath(layerFile); err != nil {
		return nil, errors.Wrapf(err, "build torrent metainfo from file '%s' failed", layerFile)
	}
	mi := &metainfo.MetaInfo{
		InfoBytes: bencode.MustMarshal(info),
	}
	logger.InfoContextf(ctx, "load torrent metainfor from file '%s' success", layerFile)
	mi.AnnounceList = [][]string{{th.op.TorrentConfig.Announce}}
	to, err := th.client.AddTorrent(mi)
	if err != nil {
		return nil, errors.Wrapf(err, "add torrent to metainfo failed")
	}
	if err = to.MergeSpec(&torrent.TorrentSpec{
		DisplayName: digest,
	}); err != nil {
		return nil, errors.Wrapf(err, "merge torrent spec failed")
	}
	logger.InfoContextf(ctx, "waiting for torrent to be loaded")
	<-to.GotInfo()
	logger.InfoContextf(ctx, "torrent info loaded, size: %d", to.Length())
	to.AddTrackers([][]string{{th.op.TorrentConfig.Announce}})
	if err = to.VerifyDataContext(ctx); err != nil {
		return nil, errors.Wrapf(err, "verify torrent data failed")
	}
	logger.InfoContextf(ctx, "generate torrent success")
	return to, nil
}

// CheckTorrentLocalExist check torrent local exist
func (th *TorrentHandler) CheckTorrentLocalExist(ctx context.Context, digest string) (*torrent.Torrent, string) {
	torrentObjs, torrentStrings := th.returnLocalTorrents(ctx)
	return torrentObjs[digest], torrentStrings[digest]
}

func (th *TorrentHandler) returnLocalTorrents(ctx context.Context) (map[string]*torrent.Torrent, map[string]string) {
	ts := th.client.Torrents()
	torrentObjs := make(map[string]*torrent.Torrent)
	torrentStrings := make(map[string]string)
	for _, t := range ts {
		if t == nil {
			continue
		}
		ti := t.Info()
		if ti == nil {
			continue
		}
		mi := t.Metainfo()
		var buffer bytes.Buffer
		if err := mi.Write(&buffer); err != nil {
			logger.ErrorContextf(ctx, "torrent get bytes failed: %s", err.Error())
			continue
		}
		digest := strings.TrimSuffix(ti.Name, ".tar.gzip")
		torrentObjs[digest] = t
		torrentStrings[digest] = base64.StdEncoding.EncodeToString(buffer.Bytes())
	}
	return torrentObjs, torrentStrings
}

func (th *TorrentHandler) gotTorrentInfo(t *torrent.Torrent) error {
	n := (t.Length() / 10000000) + 1
	gotInfoTimeout := time.After(time.Duration(n*60) * time.Second)
	select {
	case <-gotInfoTimeout:
		return errors.Errorf("got torrent info timeout")
	case <-t.GotInfo():
		return nil
	}
}

func (th *TorrentHandler) DownloadTorrent(ctx context.Context, digest, torrentBase64, targetPath string) error {
	err := th.handleDownloadTorrent(ctx, digest, torrentBase64, targetPath)
	if err != nil {
		metrics.TorrentOperationsTotal.WithLabelValues("download", "error").Inc()
	} else {
		metrics.TorrentOperationsTotal.WithLabelValues("download", "success").Inc()
	}
	return err
}

// DownloadTorrent download the file by torrent
func (th *TorrentHandler) handleDownloadTorrent(ctx context.Context, digest, torrentBase64, targetPath string) error {
	if err := th.downloadTorrent(ctx, digest, torrentBase64); err != nil {
		return err
	}
	torrentFile := path.Join(th.op.StorageConfig.TorrentPath, utils.LayerFileName(digest))
	logical, physical, isSparse, err := utils.IsSparseFile(torrentFile)
	if err != nil {
		return errors.Wrapf(err, "check sparse file failed")
	}
	if isSparse {
		return errors.Errorf("file '%s' is sparse file, logical: %d, physical: %d",
			torrentFile, logical, physical)
	}
	logger.InfoContextf(ctx, "torrent file '%s' is normal, logical: %d, physical: %d",
		torrentFile, logical, physical)
	if err = utils.CopyFile(torrentFile, targetPath); err != nil {
		return err
	}
	logger.InfoContextf(ctx, "copy torrent file %s to %s success", torrentFile, targetPath)
	return nil
}

func (th *TorrentHandler) downloadTorrent(ctx context.Context, digest, torrentBase64 string) error {
	torrentBytes, err := base64.StdEncoding.DecodeString(torrentBase64)
	if err != nil {
		return errors.Wrapf(err, "base64 decode '%s' failed", torrentBase64)
	}
	mi, err := metainfo.Load(bytes.NewBuffer(torrentBytes))
	if err != nil {
		return errors.Wrapf(err, "load metainfo '%s' failed", torrentBase64)
	}
	t, err := th.client.AddTorrent(mi)
	if err != nil {
		return errors.Wrapf(err, "add torrent '%s' failed", torrentBase64)
	}
	if err = th.gotTorrentInfo(t); err != nil {
		return err
	}

	// ignore chunk error
	t.SetOnWriteChunkError(func(err error) {})
	th.semaphore <- struct{}{}
	defer func() { <-th.semaphore }()
	t.DownloadAll()
	logger.InfoContextf(ctx, "torrent start downloading")
	start := time.Now()
	done := make(chan struct{})
	//go func() {
	//	defer close(done)
	//	waitForPieces(ctx, t, 0, t.NumPieces())
	//}()

	interval := 5 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	recorderTicker := time.NewTicker(30 * time.Second)
	defer recorderTicker.Stop()
	var currentBytes, prevBytes int64
	completedSlice := make([]int64, 0)
	allSize := formatutils.FormatSize(t.Length())
	for {
		currentBytes = t.BytesCompleted()
		byteRate := (currentBytes - prevBytes) * int64(time.Second) / int64(interval)
		select {
		case <-ticker.C:
			logger.InfoContextf(ctx, "torrent downloading(%v): %s/%s: %v/s, completed(bytes): %.2f%%",
				time.Since(start),
				formatutils.FormatSize(currentBytes),
				allSize,
				formatutils.FormatSize(byteRate),
				float64(currentBytes)/float64(t.Length())*100,
			)
			if currentBytes == t.Length() {
				close(done)
				break
			}
			completedSlice = append(completedSlice, currentBytes)
			prevBytes = currentBytes

			if currentBytes == 0 {
				noDownloadPoints := 36
				// Look at data from 36 points ago (180s) to see if still no download after ~3min
				if len(completedSlice) > noDownloadPoints {
					oldPieces := completedSlice[len(completedSlice)-noDownloadPoints]
					if currentBytes == oldPieces {
						return errors.Errorf("torrent start download failed for a long time " +
							"with can reverse")
					}
				}
			} else {
				noSpeedPoints := 12
				// Look at data from 12 points ago (60s) to see if no speed for ~1min
				if len(completedSlice) > noSpeedPoints {
					oldPieces := completedSlice[len(completedSlice)-noSpeedPoints]
					if currentBytes == oldPieces {
						return errors.Errorf("download torrent no speed")
					}
				}
			}
		case <-recorderTicker.C:
		case <-ctx.Done():
			return errors.Errorf("download torrent context exceeded")
		case <-done:
			logger.InfoContextf(ctx, "torrent download completed")
			return nil
		}
	}
}
