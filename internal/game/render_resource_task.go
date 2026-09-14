package game

// Source decode and derivative work have separate completion gates, but the
// task itself has one lifecycle. Published tasks may still own queued Draw
// submissions; cancellation retires those submissions without republishing.
type mapRenderTaskState uint8

const (
	mapRenderTaskSources mapRenderTaskState = iota
	mapRenderTaskDerived
	mapRenderTaskPublished
	mapRenderTaskCancelled
)

func (task *mapRenderPrewarmTask) isCancelled() bool {
	return task == nil || task.state == mapRenderTaskCancelled
}

func (r *Renderer) mapRenderTaskCurrent(task *mapRenderPrewarmTask) bool {
	return r != nil && !task.isCancelled() && task.generation == r.mapRenderGeneration &&
		(task.world == nil || r.game != nil && task.world == r.game.world)
}

// Drop by task identity, not map name: an obsolete generation must not cancel
// submissions made by its replacement for the same logical region.
func (r *Renderer) dropMapRenderTaskSubmissions(task *mapRenderPrewarmTask) {
	if r == nil || task == nil {
		return
	}
	uploads := r.mapRenderUploadQueue
	kept := uploads[:0]
	for _, upload := range uploads {
		if upload.task == task {
			delete(r.mapRenderUploadQueued, upload.image)
			continue
		}
		kept = append(kept, upload)
	}
	clear(uploads[len(kept):])
	r.mapRenderUploadQueue = kept
	tasks := r.mapRenderShaderWarmTasks
	retained := tasks[:0]
	for _, candidate := range tasks {
		if candidate != task {
			retained = append(retained, candidate)
		}
	}
	clear(tasks[len(retained):])
	r.mapRenderShaderWarmTasks = retained
}
