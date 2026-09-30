package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// A frame decoder revisits the MP4 index on every seek. Cache bounded blocks so
// repeated marks share that traffic, and fetch the next block only on demand.
const rangeBlock = int64(256 * 1024)

type blockCache struct {
	mu       sync.Mutex
	blocks   map[int64][]byte
	total    int64
	interval time.Duration
	last     time.Time
}

func (p *rangeProxy) block(ctx context.Context, offset int64) ([]byte, int64, error) {
	c := p.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if b, ok := c.blocks[offset]; ok {
		return b, c.total, nil
	}
	if delay := c.interval - time.Since(c.last); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	end := offset + rangeBlock - 1
	if c.total > 0 && end >= c.total {
		end = c.total - 1
	}
	p.mu.Lock()
	remaining := p.budget - p.received
	p.mu.Unlock()
	if remaining <= 0 {
		return nil, 0, errors.New("traffic byte budget exhausted")
	}
	if end-offset+1 > remaining {
		end = offset + remaining - 1
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.source, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
	req.Header.Set("Accept-Encoding", "identity")
	c.last = time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var first, last, total int64
	n, scanErr := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &total)
	if resp.StatusCode != http.StatusPartialContent || scanErr != nil || n != 3 || first != offset || last < first || last >= total || last > end || resp.ContentLength != last-first+1 || (c.total > 0 && c.total != total) {
		return nil, 0, fmt.Errorf("server did not respect bounded Range: HTTP %d, Content-Range %q", resp.StatusCode, resp.Header.Get("Content-Range"))
	}
	// A shorter block is valid only at EOF or at the explicit traffic cap.
	if last != end && last != total-1 {
		return nil, 0, errors.New("incomplete byte range")
	}
	b, readErr := io.ReadAll(io.LimitReader(resp.Body, last-first+1))
	p.mu.Lock()
	p.received += int64(len(b))
	p.mu.Unlock()
	if readErr != nil {
		return nil, 0, readErr
	}
	if int64(len(b)) != last-first+1 {
		return nil, 0, io.ErrUnexpectedEOF
	}
	c.total = total
	c.blocks[offset] = b
	return b, total, nil
}

func (p *rangeProxy) serveCached(w http.ResponseWriter, r *http.Request, start, end int64) {
	offset := start / rangeBlock * rangeBlock
	b, total, err := p.block(r.Context(), offset)
	if err != nil {
		if r.Context().Err() == nil && p.ctx.Err() == nil {
			p.fail(err)
		}
		return
	}
	if start >= total {
		p.fail(errors.New("range starts after end of source"))
		return
	}
	if end < 0 || end >= total {
		end = total - 1
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusPartialContent)
	for start <= end {
		within := start - offset
		if within >= int64(len(b)) {
			p.fail(errors.New("traffic byte budget exhausted"))
			return
		}
		count := int64(len(b)) - within
		if count > end-start+1 {
			count = end - start + 1
		}
		if _, err := w.Write(b[within : within+count]); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		start += count
		if start > end || r.Context().Err() != nil {
			return
		}
		offset = start / rangeBlock * rangeBlock
		b, _, err = p.block(r.Context(), offset)
		if err != nil {
			if r.Context().Err() == nil && p.ctx.Err() == nil {
				p.fail(err)
			}
			return
		}
	}
}
