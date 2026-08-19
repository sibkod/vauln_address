package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"vauln-address/internal/models"
	"vauln-address/internal/repository"
	"vauln-address/internal/validators"
)

// ErrQueueFull is returned when the wallet import queue has no capacity
var ErrQueueFull = errors.New("wallet import queue is full")

// jobTTL is how long finished jobs are kept for status polling
const jobTTL = 10 * time.Minute

// WalletJob is a single queued wallet-import request
type WalletJob struct {
	ID        string
	Request   models.AddWalletRequest
	Status    string
	Result    *models.AddWalletResponse
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
	done      chan struct{}
}

// Done returns a channel that is closed when the job finishes
func (j *WalletJob) Done() <-chan struct{} {
	return j.done
}

// WalletQueue serializes wallet imports through a single background worker
// that writes accumulated jobs to the database in batches (one transaction
// per batch). This removes write-lock contention on SQLite and reduces the
// number of transactions on MySQL.
type WalletQueue struct {
	repo          *repository.Repository
	incoming      chan *WalletJob
	batchSize     int
	flushInterval time.Duration

	mu   sync.RWMutex
	jobs map[string]*WalletJob

	stopCh  chan struct{}
	stopped chan struct{}
}

func NewWalletQueue(repo *repository.Repository, batchSize int, flushInterval time.Duration, queueSize int) *WalletQueue {
	if batchSize < 1 {
		batchSize = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	return &WalletQueue{
		repo:          repo,
		incoming:      make(chan *WalletJob, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
		jobs:          make(map[string]*WalletJob),
		stopCh:        make(chan struct{}),
		stopped:       make(chan struct{}),
	}
}

// Enqueue adds a request to the queue and returns its job
func (q *WalletQueue) Enqueue(req models.AddWalletRequest) (*WalletJob, error) {
	job := &WalletJob{
		ID:        uuid.NewString(),
		Request:   req,
		Status:    models.WalletJobPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		done:      make(chan struct{}),
	}
	q.mu.Lock()
	q.jobs[job.ID] = job
	q.mu.Unlock()

	select {
	case q.incoming <- job:
		return job, nil
	default:
		q.mu.Lock()
		delete(q.jobs, job.ID)
		q.mu.Unlock()
		return nil, ErrQueueFull
	}
}

// GetJob returns a snapshot of a job by ID
func (q *WalletQueue) GetJob(id string) (WalletJob, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	job, ok := q.jobs[id]
	if !ok {
		return WalletJob{}, false
	}
	return *job, true
}

// Start launches the background batch worker
func (q *WalletQueue) Start() {
	go q.run()
	log.Printf("Wallet import queue started (batch_size=%d, flush_interval=%s)", q.batchSize, q.flushInterval)
}

// Stop drains the queue and waits for the worker to finish
func (q *WalletQueue) Stop() {
	close(q.stopCh)
	<-q.stopped
}

func (q *WalletQueue) run() {
	defer close(q.stopped)

	ticker := time.NewTicker(q.flushInterval)
	defer ticker.Stop()
	cleanup := time.NewTicker(time.Minute)
	defer cleanup.Stop()

	var batch []*WalletJob
	flush := func() {
		if len(batch) == 0 {
			return
		}
		q.processBatch(batch)
		batch = nil
	}

	for {
		select {
		case job := <-q.incoming:
			batch = append(batch, job)
			if len(batch) >= q.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-cleanup.C:
			q.pruneJobs()
		case <-q.stopCh:
			// Drain whatever is still buffered, then flush everything
			for {
				select {
				case job := <-q.incoming:
					batch = append(batch, job)
				default:
					flush()
					return
				}
			}
		}
	}
}

// pruneJobs removes finished jobs older than jobTTL
func (q *WalletQueue) pruneJobs() {
	q.mu.Lock()
	defer q.mu.Unlock()
	cutoff := time.Now().Add(-jobTTL)
	for id, job := range q.jobs {
		if (job.Status == models.WalletJobDone || job.Status == models.WalletJobFailed) && job.UpdatedAt.Before(cutoff) {
			delete(q.jobs, id)
		}
	}
}

type pendingItem struct {
	jobIdx int
	item   repository.BatchWalletItem
}

// processBatch validates all jobs in the batch and writes the valid wallets
// to the database in one transaction, then resolves every job.
func (q *WalletQueue) processBatch(jobs []*WalletJob) {
	for _, job := range jobs {
		job.Result = &models.AddWalletResponse{}
		q.setStatus(job, models.WalletJobProcessing)
	}

	var items []repository.BatchWalletItem
	var refs []pendingItem

	for jobIdx, job := range jobs {
		req := job.Request
		for chain, address := range req.Addresses {
			address = strings.TrimSpace(address)
			chain = strings.ToLower(strings.TrimSpace(chain))

			if !models.IsValidChain(chain) {
				job.Result.SkippedWallets = append(job.Result.SkippedWallets, models.SkippedWallet{
					Address: address, Chain: chain, Reason: "invalid chain",
				})
				continue
			}
			if valid, _ := validators.ValidateAddress(chain, address); !valid {
				job.Result.SkippedWallets = append(job.Result.SkippedWallets, models.SkippedWallet{
					Address: address, Chain: chain, Reason: "invalid address format",
				})
				continue
			}
			items = append(items, repository.BatchWalletItem{
				Address:    address,
				Chain:      chain,
				Status:     req.Status,
				SeedPhrase: req.SeedPhrase,
				Reason:     req.Reason,
				Source:     req.Source,
			})
			refs = append(refs, pendingItem{jobIdx: jobIdx})
		}
	}

	if len(items) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		seeds, results, err := q.repo.BatchAddWallets(ctx, items)
		cancel()
		if err != nil {
			log.Printf("Wallet batch of %d job(s) failed: %v", len(jobs), err)
			for _, job := range jobs {
				q.failJob(job, err)
			}
			return
		}

		for i, res := range results {
			job := jobs[refs[i].jobIdx]
			item := items[i]
			if res.Skipped {
				job.Result.SkippedWallets = append(job.Result.SkippedWallets, models.SkippedWallet{
					Address: item.Address, Chain: item.Chain, Reason: res.SkipReason,
				})
				continue
			}
			job.Result.WalletIDs = append(job.Result.WalletIDs, res.WalletID)
		}

		// Resolve seed info per job
		for _, job := range jobs {
			phrase := job.Request.SeedPhrase
			if phrase == "" {
				continue
			}
			if info, ok := seeds[phrase]; ok {
				seedID := info.ID
				job.Result.SeedID = &seedID
				job.Result.SeedSkipped = info.Existed
			}
		}
	}

	for _, job := range jobs {
		res := job.Result
		res.WalletsAdded = len(res.WalletIDs)
		res.WalletsSkipped = len(res.SkippedWallets)
		res.Success = res.WalletsAdded > 0
		res.Message = fmt.Sprintf("Added %d wallet(s), skipped %d", res.WalletsAdded, res.WalletsSkipped)
		q.finishJob(job)
	}
}

func (q *WalletQueue) setStatus(job *WalletJob, status string) {
	q.mu.Lock()
	job.Status = status
	job.UpdatedAt = time.Now()
	q.mu.Unlock()
}

func (q *WalletQueue) finishJob(job *WalletJob) {
	q.setStatus(job, models.WalletJobDone)
	close(job.done)
}

func (q *WalletQueue) failJob(job *WalletJob, err error) {
	q.mu.Lock()
	job.Status = models.WalletJobFailed
	job.Error = err.Error()
	job.UpdatedAt = time.Now()
	q.mu.Unlock()
	close(job.done)
}
