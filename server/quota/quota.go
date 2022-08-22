// Copyright 2022 Tigris Data, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package quota

import (
	"context"
	"sync"
	"time"

	"github.com/tigrisdata/tigris/schema"

	ulog "github.com/tigrisdata/tigris/util/log"

	api "github.com/tigrisdata/tigris/api/server/v1"
	"github.com/tigrisdata/tigris/server/config"
	"github.com/tigrisdata/tigris/server/metadata"
	"github.com/tigrisdata/tigris/server/metrics"
	"github.com/tigrisdata/tigris/server/transaction"
	"go.uber.org/atomic"
	"golang.org/x/time/rate"
)

var (
	ErrRateExceeded        = api.Errorf(api.Code_RESOURCE_EXHAUSTED, "request rate limit exceeded")
	ErrThroughputExceeded  = api.Errorf(api.Code_RESOURCE_EXHAUSTED, "request throughput limit exceeded")
	ErrStorageSizeExceeded = api.Errorf(api.Code_RESOURCE_EXHAUSTED, "data size limit exceeded")
)

type State struct {
	Rate               *rate.Limiter
	WriteThroughput    *rate.Limiter
	ReadThroughput     *rate.Limiter
	Size               atomic.Int64
	SizeUpdateAt       atomic.Int64
	TenantSizeUpdateAt atomic.Int64
	SizeLock           sync.Mutex
	TenantSizeLock     sync.Mutex
}

type Manager struct {
	tenantQuota sync.Map
	cfg         *config.QuotaConfig
	tenantMgr   *metadata.TenantManager
	txMgr       *transaction.Manager
}

var mgr Manager

func Init(t *metadata.TenantManager, tx *transaction.Manager, c *config.QuotaConfig) {
	mgr = *newManager(t, tx, c)
}

// Allow checks rate, write throughput and storage size limits for the namespace
// and returns error if at least one of them is exceeded
func Allow(ctx context.Context, namespace string, reqSize int) error {
	// Emit size metrics regardless of enabled quota
	mgr.updateTenantMetrics(ctx, namespace)

	if !config.DefaultConfig.Quota.Enabled {
		return nil
	}
	return mgr.check(ctx, namespace, reqSize)
}

func newManager(t *metadata.TenantManager, tx *transaction.Manager, c *config.QuotaConfig) *Manager {
	return &Manager{cfg: c, tenantMgr: t, txMgr: tx}
}

// GetState returns quota state of the given namespace
func GetState(namespace string) *State {
	return mgr.getState(namespace)
}

func (m *Manager) getState(namespace string) *State {
	is, ok := m.tenantQuota.Load(namespace)
	if !ok {
		// Create new state if didn't exist before
		is = &State{
			Rate:            rate.NewLimiter(rate.Limit(m.cfg.RateLimit), 10),
			WriteThroughput: rate.NewLimiter(rate.Limit(m.cfg.WriteThroughputLimit), m.cfg.WriteThroughputLimit),
			ReadThroughput:  rate.NewLimiter(rate.Limit(m.cfg.ReadThroughputLimit), m.cfg.ReadThroughputLimit),
		}
		m.tenantQuota.Store(namespace, is)
	}

	return is.(*State)
}

func (m *Manager) check(ctx context.Context, namespace string, size int) error {
	s := m.getState(namespace)

	if !s.Rate.Allow() {
		return ErrRateExceeded
	}

	if !s.WriteThroughput.AllowN(time.Now(), size) {
		return ErrThroughputExceeded
	}

	return m.checkStorage(ctx, namespace, s, size)
}

func getDbSize(ctx context.Context, tenant *metadata.Tenant, db *metadata.Database) int64 {
	dbSize, err := tenant.DatabaseSize(ctx, db)
	if err != nil {
		ulog.E(err)
	}
	return dbSize
}

func getCollSize(ctx context.Context, tenant *metadata.Tenant, db *metadata.Database, coll *schema.DefaultCollection) int64 {
	collSize, err := tenant.CollectionSize(ctx, db, coll)
	if err != nil {
		ulog.E(err)
	}
	return collSize
}

func (m *Manager) updateTenantSize(ctx context.Context, namespace string) {
	if m.txMgr == nil {
		return
	}
	tenant, err := m.tenantMgr.GetTenant(ctx, namespace, m.txMgr)
	if err != nil {
		ulog.E(err)
		// Could not determine tenant, just exit
		return
	}

	for _, dbName := range tenant.ListDatabases(ctx) {
		db, err := tenant.GetDatabase(ctx, dbName)
		if err != nil {
			ulog.E(err)
			return
		}
		metrics.UpdateDbSizeMetrics(namespace, dbName, getDbSize(ctx, tenant, db))
		for _, coll := range db.ListCollection() {
			metrics.UpdateCollectionSizeMetrics(namespace, dbName, coll.Name, getCollSize(ctx, tenant, db, coll))
		}
	}
	tenantSize, err := tenant.Size(ctx)
	if err != nil {
		ulog.E(err)
	}
	metrics.UpdateNameSpaceSizeMetrics(namespace, tenantSize)
}

func (m *Manager) updateTenantMetrics(ctx context.Context, namespace string) {
	s := m.getState(namespace)
	sz := s.Size.Load()
	currentTimeStamp := time.Now().Unix()

	if currentTimeStamp >= s.TenantSizeUpdateAt.Load()+m.cfg.TenantSizeRefreshInterval {
		s.TenantSizeLock.Lock()
		defer s.TenantSizeLock.Unlock()

		s.TenantSizeUpdateAt.Store(currentTimeStamp)
		metrics.UpdateNameSpaceSizeMetrics(namespace, sz)
		m.updateTenantSize(ctx, namespace)
	}
}

func (m *Manager) checkStorage(ctx context.Context, namespace string, s *State, size int) error {
	sz := s.Size.Load()
	currentTimeStamp := time.Now().Unix()

	if currentTimeStamp < s.SizeUpdateAt.Load()+m.cfg.LimitUpdateInterval {
		if sz+int64(size) >= m.cfg.DataSizeLimit {
			return ErrStorageSizeExceeded
		}
		return nil
	}

	s.SizeLock.Lock()
	defer s.SizeLock.Unlock()

	if currentTimeStamp >= s.SizeUpdateAt.Load()+m.cfg.LimitUpdateInterval {
		s.SizeUpdateAt.Store(currentTimeStamp)

		t, err := m.tenantMgr.GetTenant(ctx, namespace, m.txMgr)
		if err != nil {
			return err
		}

		dsz, err := t.Size(ctx)
		if err != nil {
			return err
		}

		s.Size.Store(dsz)
	}

	sz = s.Size.Load()

	if sz+int64(size) >= m.cfg.DataSizeLimit {
		return ErrStorageSizeExceeded
	}

	return nil
}