package main

// This program acts as a task queue manager.

import (
	"database/sql"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/lib/pq"
)

// Task is a struct that holds an entry in a task queue.
// The queue itself is stored in a database table.
// Other parts of the system can create tasks by inserting rows.
// A task will be retried until it succeeds, and then discarded.
// A task has an url. To run a task, a http get request will be performed.
// A return status of 200 is interpreted as success, everything else is a failure.
// In case of failure, the task will be retried after a while.
// An exception is http status 410 which is interpreted as a permanent failure
// that is pointless to retry.
type Task struct {
	taskid  int64
	url     string
	lasttry time.Time
	status  int
	delay   int
	delay2  int
}

var mu sync.RWMutex
var runningTasks = make(map[int64]bool)

func isTaskRunning(id int64) bool {
	mu.RLock()
	defer mu.RUnlock()
	return runningTasks[id]
}
func markTaskRunning(id int64) {
	mu.Lock()
	defer mu.Unlock()
	runningTasks[id] = true
}
func markTaskDone(id int64) {
	mu.Lock()
	defer mu.Unlock()
	delete(runningTasks, id)
}

// taskRunner is intended to be run as a goroutine. It enters an infinite loop
// where it periodically reads the "task" database table and executes tasks.
func taskRunner(db *sql.DB, devmode bool) {
	taskSlots := make(chan bool, 10) // max concurrent running tasks
	for {
		// Read the current active tasks from the database
		rows, err := db.Query("SELECT taskid, url, lasttry, " +
			"status, delay, delay2 FROM tasks")
		if err != nil {
			log.Panic(err)
		}
		tasks := make([]Task, 0, 0)
		for rows.Next() {
			var task Task
			var taskurl sql.NullString
			var timestamp pq.NullTime
			err = rows.Scan(&task.taskid, &taskurl, &timestamp,
				&task.status, &task.delay, &task.delay2)
			if err != nil {
				log.Panic(err)
			}
			if isTaskRunning(task.taskid) {
				continue
			}
			if taskurl.Valid {
				task.url = taskurl.String
			}
			if timestamp.Valid {
				task.lasttry = timestamp.Time
			}
			tasks = append(tasks, task)
		}
		if rows.Err() != nil {
			log.Panic(rows.Err())
		}
		rows.Close()

		// Find tasks that should be run/re-tried right now
		canWait := 20
		for _, task := range tasks {
			if task.lasttry.IsZero() ||
				time.Since(task.lasttry).Seconds() > float64(task.delay) {
				taskSlots <- true // this will block until there's a free slot
				markTaskRunning(task.taskid)
				go func(task Task) {
					defer func() {
						markTaskDone(task.taskid)
						<-taskSlots // free up a task slot
					}()
					executeTask(db, task)
				}(task)
			} else {
				timeleft := task.delay - int(time.Since(task.lasttry).Seconds())
				if timeleft < canWait {
					canWait = timeleft
				}
			}
		}

		// Sleep
		freeSlots := len(taskSlots)
		for second := 0; second < canWait; second++ {
			time.Sleep(time.Second)
			if freeSlots != len(taskSlots) {
				// A task finished. Stop sleeping and see if there's more work to do now
				break
			}
		}
	}
}

func executeTask(db *sql.DB, task Task) {
	resp, err := http.Get(task.url)
	if err == nil {
		task.status = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode == 200 || resp.StatusCode == 410 {
			db.Exec("DELETE FROM tasks WHERE taskid=$1", task.taskid)
			return
		}
	} else {
		task.status = 1
	}
	task.lasttry = time.Now()

	// Fibonacci sequence determines the delay in seconds
	if task.delay == 0 && task.delay2 == 0 {
		task.delay = 1
		task.delay2 = 0
	} else {
		newdelay := task.delay + task.delay2
		task.delay2 = task.delay
		task.delay = newdelay
	}
	// Max delay is 24 hours.
	if task.delay > 86400 {
		task.delay = 86400
	}

	db.Exec("UPDATE tasks SET lasttry=$1, delay=$2, delay2=$3, status=$4 "+
		" WHERE taskid=$5",
		task.lasttry, task.delay, task.delay2, task.status, task.taskid)
}
