package scheduler

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"time"

	kademliadfs "github.com/hasocobo/kademlia-dfs/kademlia"
	"github.com/hasocobo/kademlia-dfs/runtime"
)

const (
	execWorkerPoolSize   = 8
	resultWorkerPoolSize = 8
)

type Worker struct {
	exec    runtime.TaskRuntime
	network runtime.TaskNetwork

	streamTasks map[JobID]struct{} // TODO: replace struct{} with stream lifecycle state
	maxStreams  int

	batchTasks   chan runtime.Task
	batchResults chan ExecResult

	mountDir     string
	capabilities []string
	tags         []string

	topicPublisher TopicPublisher
}

type TopicPublisher interface {
	Put(ctx context.Context, key string, value []byte) error
}

func NewWorker(exec runtime.TaskRuntime, network runtime.TaskNetwork,
	mountDir string,
	capabilities []string,
	tags []string,
) *Worker {
	return &Worker{
		exec:         exec,
		network:      network,
		streamTasks:  make(map[JobID]struct{}),
		batchTasks:   make(chan runtime.Task, execWorkerPoolSize),
		batchResults: make(chan ExecResult, resultWorkerPoolSize),
		mountDir:     mountDir,
		capabilities: capabilities,
		tags:         tags,
		maxStreams:   1,
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Printf("worker starting")
	w.startWorkers(ctx, execWorkerPoolSize, resultWorkerPoolSize)
	go w.fetchTasks(ctx)
}

func (w *Worker) RegisterTopicPublisher(publisher TopicPublisher) {
	w.topicPublisher = publisher
}

type ExecResult struct {
	TaskID TaskID
	Result []byte
}

func (w *Worker) startWorkers(ctx context.Context, execPoolSize int, resultPoolSize int) {
	for range execPoolSize {
		go w.execTask(ctx)
	}
	for range resultPoolSize {
		go w.writeTaskResult(ctx)
	}
}

func (w *Worker) execTask(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.batchTasks:
			if len(task.Binary) == 0 {
				log.Println("execTask: task binary is empty")
				return
			}
			result, err := w.exec.RunTask(ctx, task.Binary, task.Stdin, io.Discard)
			if err != nil {
				log.Printf("execTask: error: %v", err)
				continue
			}
			w.batchResults <- ExecResult{TaskID: task.TaskID, Result: result}
		}
	}
}

func (w *Worker) writeTaskResult(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-w.batchResults:
			payload := runtime.Task{
				OpCode: kademliadfs.ExecutionResponse,
				TaskID: result.TaskID,
				Result: result.Result,
			}
			encodedMessage, err := runtime.EncodeTask(payload)
			if err != nil {
				log.Printf("writeTaskResult: error encoding: %v", err)
				continue
			}
			addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9000}
			if _, err := w.network.SendTask(ctx, encodedMessage, addr); err != nil {
				log.Printf("writeTaskResult: error sending result: %v", err)
			}
		}
	}
}

func (w *Worker) fetchTasks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("fetchTasks: polling for available tasks")
			bootstrapNode := kademliadfs.Contact{ID: kademliadfs.NodeId{}, IP: net.IPv4(127, 0, 0, 1), Port: 9000}

			mode := runtime.RequestModeNeedBatch
			if len(w.streamTasks) < w.maxStreams {
				mode = runtime.RequestModeNeedBatchOrStream
			}
			req := runtime.Task{
				OpCode:       kademliadfs.LeaseRequest,
				Capabilities: w.capabilities,
				Tags:         w.tags,
				RequestMode:  mode,
			}

			t, err := runtime.EncodeTask(req)
			if err != nil {
				log.Printf("fetchTasks: error encoding: %v", err)
				return
			}
			addr := &net.UDPAddr{IP: bootstrapNode.IP, Port: bootstrapNode.Port}
			resp, err := w.network.RequestTask(ctx, t, addr)
			if err != nil {
				log.Printf("fetchTasks: no available task found")
				continue
			}

			task, err := runtime.DecodeTask(resp)
			if err != nil {
				log.Printf("fetchTasks: error decoding: %v", err)
				continue
			}

			switch task.Type {
			case runtime.TaskTypeBatch:
				w.batchTasks <- task
			case runtime.TaskTypeStream:
				if _, exists := w.streamTasks[task.JobID]; exists {
					log.Printf("stream task %v is already running", task.JobID)
					break
				}
				go w.startStreamTask(ctx, task)
			}
		}
	}
}

func (w *Worker) startStreamTask(ctx context.Context, task runtime.Task) error {
	w.streamTasks[task.JobID] = struct{}{}

	topicName := task.Topic
	pr, pw := io.Pipe()
	defer pw.Close()
	go w.startLogRotator(ctx, topicName, w.mountDir, pr)
	if _, err := w.exec.RunTask(ctx, task.Binary, task.Stdin, pw); err != nil {
		return err
	}

	return nil
}

// startLogRotator periodically takes a log file, renames it and publishes to DHT network under the topicName
func (w *Worker) startLogRotator(ctx context.Context, topicName string, mountDir string, pr io.Reader) {
	if topicName == "" {
		log.Printf("startLogRotator: empty topic, skipping")
		return
	}
	if w.topicPublisher == nil {
		log.Printf("startLogRotator: topic publisher not registered, skipping")
		return
	}

	openNewFile := func() (string, *os.File, error) {
		filePath := path.Join(mountDir, fmt.Sprintf("%s.%d.log", topicName, time.Now().UnixNano()))
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return "", nil, err
		}
		return filePath, f, nil
	}

	publishAndRotate := func(currentPath string, currentFile *os.File) (string, *os.File, error) {
		if syncErr := currentFile.Sync(); syncErr != nil {
			log.Printf("startLogRotator: sync failed for %q: %v", currentPath, syncErr)
		}
		if closeErr := currentFile.Close(); closeErr != nil {
			log.Printf("startLogRotator: close failed for %q: %v", currentPath, closeErr)
		}

		fileBytes, readErr := os.ReadFile(currentPath)
		if readErr != nil {
			log.Printf("startLogRotator: read rotated file %q failed: %v", currentPath, readErr)
			log.Printf("startLogRotator: rotation event topic=%q file=%q bytes=0", topicName, currentPath)
		} else if len(fileBytes) > 0 {
			fileHash := sha256.Sum256(fileBytes)
			// hex encoding restricts output to numbers and character only and avoids accidental , ; etc in the value
			hashPayload := []byte(hex.EncodeToString(fileHash[:]))
			log.Printf("startLogRotator: rotation event topic=%q file=%q bytes=%d", topicName, currentPath, len(fileBytes))
			log.Printf("startLogRotator: publishing topic=%q file=%q bytes=%d hash=%s", topicName, currentPath, len(fileBytes), string(hashPayload))
			if putErr := w.topicPublisher.Put(ctx, topicName, hashPayload); putErr != nil {
				log.Printf("startLogRotator: publish failed for topic %q: %v", topicName, putErr)
			}
		} else {
			log.Printf("startLogRotator: rotation event topic=%q file=%q bytes=0", topicName, currentPath)
		}

		nextPath, nextFile, openErr := openNewFile()
		if openErr != nil {
			return "", nil, openErr
		}
		return nextPath, nextFile, nil
	}

	currentPath, currentFile, err := openNewFile()
	if err != nil {
		log.Printf("startLogRotator: failed creating active log file for topic %q: %v", topicName, err)
		return
	}
	defer currentFile.Close()

	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if scanErr := scanner.Err(); scanErr != nil {
			log.Printf("startLogRotator: scanner error for topic %q: %v", topicName, scanErr)
		}
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case line, exists := <-lines:
			if !exists {
				publishAndRotate(currentPath, currentFile)
				return
			}
			if _, writeErr := currentFile.WriteString(line + "\n"); writeErr != nil {
				log.Printf("startLogRotator: failed writing active file %q: %v", currentPath, writeErr)
			}

		case <-ticker.C:
			nextPath, nextFile, openErr := publishAndRotate(currentPath, currentFile)
			if openErr != nil {
				log.Printf("startLogRotator: failed rotating log file for topic %q: %v", topicName, openErr)
				return
			}
			currentPath = nextPath
			currentFile = nextFile
		}
	}
}
