package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qa-automation/ai-backend/internal/services/objectstore"
	"golang.org/x/sync/singleflight"
)

// Export cache (MODULE_CHECKLIST_EXPORT.md, "Request flow").

// forceCooldown: a forced request is ignored when the stored object is younger than this.
const forceCooldown = 30 * time.Second

// generateTimeout is the ceiling of one generation (snapshot plus three renders).
const generateTimeout = 60 * time.Second

// Values of ExportResult.Cache (the X-Export-Cache header).
const (
	CacheHit         = "hit"
	CacheMiss        = "miss"
	CacheRegenerated = "regenerated"
	CacheBypass      = "bypass"
)

var exportFormats = []string{"pdf", "xlsx", "csv"}

// ExportQuery is a validated export request.
type ExportQuery struct {
	Format     string // pdf, xlsx or csv
	LastUpdate bool
	Force      bool
	Filter     ChecklistFilter // zero value = every feature
}

// ExportResult is what the handler streams.
type ExportResult struct {
	Body        []byte
	ContentType string
	Filename    string
	Cache       string // CacheHit, CacheMiss, CacheRegenerated or CacheBypass
}

// snapshotSource is implemented by *Syncer; tests use a fake.
type snapshotSource interface {
	ChecklistKeyInfo(ctx context.Context, pid int, withHistory bool) (KeyInfo, error)
	ChecklistSnapshot(ctx context.Context, pid int, withHistory bool) (*ChecklistSnapshot, error)
}

type renderFunc func(*ChecklistSnapshot, string, bool) ([]byte, string, error)

// ExportCache serves the checklist from the object store, generating it on a miss.
type ExportCache struct {
	store   objectstore.Store // never nil
	ns      string            // MINIO_KEY_NAMESPACE
	src     snapshotSource
	now     func() time.Time
	render  renderFunc
	timeout time.Duration // MINIO_TIMEOUT_SECONDS
	flights singleflight.Group
	bg      sync.WaitGroup // background Puts
}

// NewExportCache builds the cache. store must not be nil and ns must be a valid namespace.
func NewExportCache(src snapshotSource, store objectstore.Store, ns string, timeout time.Duration) *ExportCache {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &ExportCache{store: store, ns: ns, src: src, now: time.Now, render: RenderChecklist, timeout: timeout}
}

// Wait blocks until the background Puts have finished (tests and shutdown).
func (c *ExportCache) Wait() { c.bg.Wait() }

var nameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// ExportBaseName makes a product name safe for a header and a path: every run of characters
// outside [A-Za-z0-9._-] becomes '-', leading and trailing '-' are trimmed, and the result is cut
// to 60 characters. An empty result becomes "product-<id>".
func ExportBaseName(name string, pid int) string {
	s := strings.Trim(nameUnsafe.ReplaceAllString(name, "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		return fmt.Sprintf("product-%d", pid)
	}
	return s
}

func nameHash(base string) string {
	h := sha1.Sum([]byte(base))
	return hex.EncodeToString(h[:])[:8]
}

var formatExt = map[string]bool{"pdf": true, "xlsx": true, "csv": true}

// exportDir is the object directory (the key without the extension); it is also the flight key.
func (c *ExportCache) exportDir(pid int, syncID int64, name string, lastUpdate bool, histMax int64, f ChecklistFilter) string {
	variant := "plain"
	if lastUpdate {
		variant = fmt.Sprintf("last-update-h%d", histMax)
	}
	variant += f.variant()
	return fmt.Sprintf("%s%s/knowledge-exports/%d/modules-checklist/sync-%d/v%d/n%s/%s",
		objectstore.Prefix, c.ns, pid, syncID, TemplateVersion, nameHash(ExportBaseName(name, pid)), variant)
}

func exportKey(dir, format string) string { return dir + "." + format }

func (c *ExportCache) logBypass(pid int, err error) {
	log.Printf("knowledge export: cache bypass product=%d op=%s code=%s", pid, objectstore.ErrOp(err), objectstore.ErrCode(err))
}

func contentTypeOf(format string) string {
	switch format {
	case "pdf":
		return "application/pdf"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	return "text/csv; charset=utf-8"
}

type flightOut struct {
	bodies map[string][]byte // by format
	types  map[string]string
	dir    string // the directory the objects belong to (rebuilt from the snapshot)
	reqFmt string // the format the flight's leader stored first
	putOK  bool   // the leader's object was stored
	snap   *ChecklistSnapshot
	extras sync.Once // the two other formats are stored once, by whichever waiter gets here first
}

// Serve returns the checklist file for the product.
func (c *ExportCache) Serve(ctx context.Context, pid int, q ExportQuery) (ExportResult, error) {
	if !formatExt[q.Format] {
		return ExportResult{}, fmt.Errorf("unknown export format %q", q.Format)
	}
	info, err := c.src.ChecklistKeyInfo(ctx, pid, q.LastUpdate)
	if err != nil {
		return ExportResult{}, err
	}
	base := ExportBaseName(info.ProductName, pid)
	dir := c.exportDir(pid, info.SyncID, info.ProductName, q.LastUpdate, info.HistoryMaxID, q.Filter)
	key := exportKey(dir, q.Format)
	result := func(body []byte, cache string) ExportResult {
		return ExportResult{Body: body, ContentType: contentTypeOf(q.Format), Filename: base + "-MODULE-CHECKLIST" + strings.ToUpper(strings.ReplaceAll(q.Filter.variant(), "/", "-")) + "." + q.Format, Cache: cache}
	}

	skipPuts := false
	if !q.Force {
		b, found, err := c.store.Get(ctx, key)
		switch {
		case err != nil:
			c.logBypass(pid, err)
			skipPuts = true
		case found:
			return result(b, CacheHit), nil
		}
	} else {
		in, exists, err := c.store.Stat(ctx, key)
		switch {
		case err != nil:
			c.logBypass(pid, err)
			skipPuts = true
		case exists:
			age := c.now().Sub(in.LastModified)
			if age < 0 {
				age = 0 // clock skew between the store and this host
			}
			if age < forceCooldown {
				b, found, err := c.store.Get(ctx, key)
				switch {
				case err != nil:
					c.logBypass(pid, err)
					skipPuts = true
				case found:
					return result(b, CacheHit), nil
				}
			}
		}
	}

	flightKey := dir
	if q.Force {
		flightKey += ":force"
	}
	v, ferr, _ := c.flights.Do(flightKey, func() (any, error) {
		// A client closing its tab must not fail the other waiters.
		gctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), generateTimeout)
		defer cancel()
		snap, err := c.src.ChecklistSnapshot(gctx, pid, q.LastUpdate)
		if err != nil {
			return nil, err
		}
		snap = FilterChecklist(snap, q.Filter)
		out := &flightOut{reqFmt: q.Format, bodies: map[string][]byte{}, types: map[string]string{}, dir: dir, snap: snap}
		if snap.SyncID != info.SyncID || snap.HistoryMaxID != info.HistoryMaxID || snap.ProductName != info.ProductName {
			// The snapshot is the newer truth: store under the key it was built from.
			out.dir = c.exportDir(pid, snap.SyncID, snap.ProductName, q.LastUpdate, snap.HistoryMaxID, q.Filter)
		}
		for _, f := range exportFormats {
			b, ct, err := c.render(snap, f, q.LastUpdate)
			if err != nil {
				return nil, err
			}
			out.bodies[f], out.types[f] = b, ct
		}
		if skipPuts {
			return out, nil
		}
		pctx, pcancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer pcancel()
		if err := c.store.Put(pctx, exportKey(out.dir, q.Format), out.types[q.Format], out.bodies[q.Format]); err != nil {
			c.logBypass(pid, err)
			return out, nil
		}
		out.putOK = true
		return out, nil
	})
	if ferr != nil {
		return ExportResult{}, ferr
	}
	out := v.(*flightOut)

	cache := CacheBypass
	if out.putOK {
		cache = CacheMiss
		if q.Force {
			cache = CacheRegenerated
		}
		out.extras.Do(func() { c.putOthers(pid, out) })
	}
	return result(out.bodies[q.Format], cache), nil
}

// putOthers stores the two formats that were not requested, after the response is ready, under
// one deadline of twice the per-call timeout. Errors are logged and ignored.
func (c *ExportCache) putOthers(pid int, out *flightOut) {
	c.bg.Add(1)
	go func() {
		defer c.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*c.timeout)
		defer cancel()
		for _, f := range exportFormats {
			if f == out.reqFmt {
				continue
			}
			if err := c.store.Put(ctx, exportKey(out.dir, f), out.types[f], out.bodies[f]); err != nil {
				c.logBypass(pid, err)
			}
		}
	}()
}
