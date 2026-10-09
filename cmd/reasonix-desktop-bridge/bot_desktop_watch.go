package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"reasonix/internal/bot"
	appconfig "reasonix/internal/config"
)

var errPreviewDesktopWatch = errors.New("Preview desktop watch is unavailable")

const previewDesktopWatchLimit = 128

type previewDesktopWatcher struct {
	route bot.DesktopWatchRoute
	actor string
}

// previewDesktopWatchStore survives gateway refreshes; the full host must keep
// the same instance. A failed save remains authoritative in process memory,
// never silently reseeded from an older disk snapshot. Persisting a route/actor
// is not authorization to send: consumers must recheck current adapter/access
// and admin permission for the actor before every delivery.
type previewDesktopWatchStore struct {
	mu       sync.Mutex
	watchers map[string]previewDesktopWatcher
	persist  func(previewDesktopWatcher, bool) error
	closed   bool
	leases   map[string]previewDesktopWatchLease
}

type previewDesktopWatchLease struct {
	watcher previewDesktopWatcher
	ctx     context.Context
	cancel  context.CancelFunc
}

func newPreviewDesktopWatchStore(path string) (*previewDesktopWatchStore, error) {
	cfg, err := appconfig.LoadForEditWithoutCredentialsReadOnlyStrict(path)
	if err != nil {
		return nil, err
	}
	if len(cfg.Bot.DesktopWatchers) > previewDesktopWatchLimit {
		return nil, errPreviewDesktopWatch
	}
	store := &previewDesktopWatchStore{watchers: make(map[string]previewDesktopWatcher)}
	// Ambiguous duplicate ownership is not resolved by file order. Keep old
	// actor-less Wails subscriptions on disk but never activate them in Preview.
	seen := make(map[string]bool)
	for _, record := range cfg.Bot.DesktopWatchers {
		watcher := previewWatcherFromConfig(record)
		key := watcher.route.Key()
		if seen[key] {
			delete(store.watchers, key)
			continue
		}
		seen[key] = true
		if len(store.watchers) < previewDesktopWatchLimit && validPreviewWatcher(watcher) {
			store.watchers[key] = watcher
		}
	}
	store.persist = func(watcher previewDesktopWatcher, enabled bool) error {
		// Per-route transaction preserves unrelated settings, legacy watchers,
		// and subscriptions changed by another process; no stale whole snapshot.
		return appconfig.EditConfigFileWithoutCredentials(path, func(cfg *appconfig.Config) error {
			next := make([]appconfig.BotDesktopWatcherConfig, 0, len(cfg.Bot.DesktopWatchers)+1)
			for _, record := range cfg.Bot.DesktopWatchers {
				if previewWatcherFromConfig(record).route.Key() != watcher.route.Key() {
					next = append(next, record)
				}
			}
			if enabled {
				next = append(next, previewWatcherConfig(watcher))
			}
			if len(next) > previewDesktopWatchLimit {
				return errPreviewDesktopWatch
			}
			cfg.Bot.DesktopWatchers = next
			return nil
		})
	}
	return store, nil
}

func previewWatcherFromConfig(record appconfig.BotDesktopWatcherConfig) previewDesktopWatcher {
	return previewDesktopWatcher{bot.DesktopWatchRoute{Platform: bot.Platform(record.Platform), ConnectionID: record.ConnectionID, Domain: record.Domain, ChatType: bot.ChatType(record.ChatType), ChatID: record.ChatID}, record.ActorID}
}

func previewWatcherConfig(watcher previewDesktopWatcher) appconfig.BotDesktopWatcherConfig {
	r := watcher.route
	return appconfig.BotDesktopWatcherConfig{Platform: string(r.Platform), ConnectionID: r.ConnectionID, Domain: r.Domain, ChatType: string(r.ChatType), ChatID: r.ChatID, ActorID: watcher.actor}
}

func validPreviewWatcher(watcher previewDesktopWatcher) bool {
	if !validPreviewDesktopRoute(watcher.route) || watcher.actor == "" || len(watcher.actor) > 512 {
		return false
	}
	// Render normalizes strings: reject non-canonical inbound identities rather
	// than letting restart adopt a subtly different chat or authenticated actor.
	for _, value := range []string{watcher.actor, watcher.route.ConnectionID, watcher.route.Domain, watcher.route.ChatID} {
		if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	return true
}

func (s *previewDesktopWatchStore) SetWatch(ctx context.Context, route bot.DesktopWatchRoute, actor string, enabled bool) error {
	watcher := previewDesktopWatcher{route, actor}
	if ctx == nil || ctx.Err() != nil || !validPreviewWatcher(watcher) {
		return errPreviewDesktopWatch
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx.Err() != nil {
		return errPreviewDesktopWatch
	}
	key := route.Key()
	if lease, ok := s.leases[key]; ok {
		lease.cancel()
		delete(s.leases, key)
	}
	if enabled {
		if _, existing := s.watchers[key]; !existing && len(s.watchers) >= previewDesktopWatchLimit {
			return errPreviewDesktopWatch
		}
		s.watchers[key] = watcher
	} else {
		delete(s.watchers, key)
	}
	// Serialize mutation + disk transaction. Off can never be overwritten by
	// a delayed on snapshot. No network/model IO occurs under this store lock.
	if err := s.persist(watcher, enabled); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	} // state may have changed: caller must report unknown
	return nil
}

func (s *previewDesktopWatchStore) Watching(route bot.DesktopWatchRoute) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.watchers[route.Key()]
	return !s.closed && ok
}

func (s *previewDesktopWatchStore) Snapshot() []previewDesktopWatcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	routes := make([]previewDesktopWatcher, 0, len(s.watchers))
	for _, watcher := range s.watchers {
		routes = append(routes, watcher)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].route.Key() < routes[j].route.Key() })
	return routes
}

func (s *previewDesktopWatchStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, lease := range s.leases {
		lease.cancel()
	}
	clear(s.leases)
	clear(s.watchers)
}

// deliveryLeases capture the current in-memory subscription generation. Off,
// actor replacement and Close revoke it even if persistence fails. Network IO
// never holds the store lock; cancellation cannot retract an admitted packet.
func (s *previewDesktopWatchStore) deliveryLeases() []previewDesktopWatchLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.leases == nil {
		s.leases = make(map[string]previewDesktopWatchLease)
	}
	result := make([]previewDesktopWatchLease, 0, len(s.watchers))
	for key, watcher := range s.watchers {
		lease, ok := s.leases[key]
		if !ok {
			ctx, cancel := context.WithCancel(context.Background())
			lease = previewDesktopWatchLease{watcher, ctx, cancel}
			s.leases[key] = lease
		}
		result = append(result, lease)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].watcher.route.Key() < result[j].watcher.route.Key() })
	return result
}
