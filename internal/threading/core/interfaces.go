package core

// CreateDefaultWorkerPool creates and starts a worker pool sized to
// runtime.NumCPU().
func CreateDefaultWorkerPool() *WorkerPool {
	pool := NewWorkerPool(0)
	pool.Start()
	return pool
}
