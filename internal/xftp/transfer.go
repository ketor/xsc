package xftp

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/sftp"
)

// Direction 传输方向
type Direction int

const (
	Upload   Direction = iota // 本地→远程
	Download                  // 远程→本地
)

// TaskStatus 传输任务状态
type TaskStatus int

const (
	StatusPending   TaskStatus = iota // 等待中
	StatusActive                      // 传输中
	StatusCompleted                   // 已完成
	StatusFailed                      // 失败
	StatusCancelled                   // 已取消
)

// 传输常量
const (
	transferBufSize    = 32 * 1024              // 32KB 缓冲区
	progressInterval   = 100 * time.Millisecond // 进度更新间隔
	speedWindowSeconds = 5                      // 速度计算滑动窗口（秒）
)

// TransferTask 传输任务
type TransferTask struct {
	ID          int
	Source      string
	Dest        string
	Direction   Direction
	Size        int64
	Status      TaskStatus
	Progress    float64 // 0.0 - 1.0
	Speed       float64 // bytes/sec
	Transferred int64
	Error       error
}

// TransferManager 传输管理器
type TransferManager struct {
	tasks      []TransferTask
	active     *TransferTask
	nextID     int
	cancelFn   context.CancelFunc
	progressCh chan progressUpdate
	mu         sync.Mutex

	// 传输统计（跨任务累积）
	totalFiles  int
	totalDirs   int
	totalBytes  int64
	failedCount int
}

// progressUpdate 内部进度更新结构
type progressUpdate struct {
	taskID      int
	transferred int64
	done        bool
	err         error
}

// NewTransferManager 创建传输管理器
func NewTransferManager() *TransferManager {
	return &TransferManager{
		progressCh: make(chan progressUpdate, 64),
	}
}

// AddTask 添加传输任务到队列，返回任务 ID
func (tm *TransferManager) AddTask(source, dest string, dir Direction, size int64) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := tm.nextID
	tm.nextID++

	task := TransferTask{
		ID:        id,
		Source:    source,
		Dest:      dest,
		Direction: dir,
		Size:      size,
		Status:    StatusPending,
	}
	tm.tasks = append(tm.tasks, task)
	return id
}

// StartNext 开始下一个 pending 任务，返回 tea.Cmd
// sftpClient 用于远程操作
func (tm *TransferManager) StartNext(sftpClient *sftp.Client) tea.Cmd {
	tm.mu.Lock()

	// 找到下一个 pending 任务
	var task *TransferTask
	for i := range tm.tasks {
		if tm.tasks[i].Status == StatusPending {
			task = &tm.tasks[i]
			break
		}
	}

	if task == nil {
		tm.mu.Unlock()
		return nil
	}

	task.Status = StatusActive
	tm.active = task

	ctx, cancel := context.WithCancel(context.Background())
	tm.cancelFn = cancel

	taskCopy := *task
	progressCh := tm.progressCh
	tm.mu.Unlock()

	// 启动传输 goroutine
	return func() tea.Msg {
		var err error
		if taskCopy.Direction == Upload {
			err = doUpload(ctx, taskCopy.Source, taskCopy.Dest, sftpClient, taskCopy.ID, progressCh)
		} else {
			err = doDownload(ctx, taskCopy.Source, taskCopy.Dest, sftpClient, taskCopy.ID, progressCh)
		}

		if err != nil {
			progressCh <- progressUpdate{taskID: taskCopy.ID, done: true, err: err}
		} else {
			progressCh <- progressUpdate{taskID: taskCopy.ID, done: true, transferred: taskCopy.Size}
		}

		// 返回完成/错误消息到 Bubble Tea
		if err != nil {
			return TransferErrorMsg{TaskID: taskCopy.ID, Err: err}
		}
		return TransferCompleteMsg{TaskID: taskCopy.ID}
	}
}

// Cancel 取消当前传输
func (tm *TransferManager) Cancel() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.cancelFn != nil {
		tm.cancelFn()
		tm.cancelFn = nil
	}
	if tm.active != nil {
		tm.active.Status = StatusCancelled
		tm.active = nil
	}
}

// ListenProgress 监听进度 channel 的 tea.Cmd
// 每次从 channel 读取一条进度更新并返回 TransferProgressMsg
func (tm *TransferManager) ListenProgress() tea.Cmd {
	ch := tm.progressCh
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}

		tm.mu.Lock()
		defer tm.mu.Unlock()

		// 更新任务状态
		for i := range tm.tasks {
			if tm.tasks[i].ID == update.taskID {
				if update.done {
					if update.err != nil {
						tm.tasks[i].Status = StatusFailed
						tm.tasks[i].Error = update.err
					} else {
						tm.tasks[i].Status = StatusCompleted
						tm.tasks[i].Progress = 1.0
						tm.tasks[i].Transferred = tm.tasks[i].Size
					}
					tm.active = nil
				} else {
					tm.tasks[i].Transferred = update.transferred
					if tm.tasks[i].Size > 0 {
						tm.tasks[i].Progress = float64(update.transferred) / float64(tm.tasks[i].Size)
					}
				}
				// 返回进度消息
				return TransferProgressMsg{
					TaskID:      update.taskID,
					Progress:    tm.tasks[i].Progress,
					Speed:       tm.tasks[i].Speed,
					Transferred: tm.tasks[i].Transferred,
				}
			}
		}

		return nil
	}
}

// Tasks 返回所有任务（只读快照）
func (tm *TransferManager) Tasks() []TransferTask {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	result := make([]TransferTask, len(tm.tasks))
	copy(result, tm.tasks)
	return result
}

// ActiveTask 返回当前活跃任务
func (tm *TransferManager) ActiveTask() *TransferTask {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.active == nil {
		return nil
	}
	copy := *tm.active
	return &copy
}

// HasPending 是否有等待中的任务
func (tm *TransferManager) HasPending() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, t := range tm.tasks {
		if t.Status == StatusPending {
			return true
		}
	}
	return false
}

// ClearCompleted 清除已完成的任务
func (tm *TransferManager) ClearCompleted() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var remaining []TransferTask
	for _, t := range tm.tasks {
		if t.Status != StatusCompleted && t.Status != StatusFailed && t.Status != StatusCancelled {
			remaining = append(remaining, t)
		}
	}
	tm.tasks = remaining
}

// ResetStats 重置传输统计
func (tm *TransferManager) ResetStats() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.totalFiles = 0
	tm.totalDirs = 0
	tm.totalBytes = 0
	tm.failedCount = 0
}

// Stats 返回传输统计
func (tm *TransferManager) Stats() (files, dirs int, bytes int64, failed int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.totalFiles, tm.totalDirs, tm.totalBytes, tm.failedCount
}

// RecordFileComplete 记录一个文件传输完成
func (tm *TransferManager) RecordFileComplete(size int64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.totalFiles++
	tm.totalBytes += size
}

// RecordDirComplete 记录目录传输完成的数量
func (tm *TransferManager) RecordDirComplete(count int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.totalDirs += count
}

// RecordFailed 记录一个传输失败
func (tm *TransferManager) RecordFailed() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.failedCount++
}

// doUpload 执行上传：本地文件 → 远程
func doUpload(ctx context.Context, src, dest string, client *sftp.Client, taskID int, ch chan<- progressUpdate) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer srcFile.Close()

	// 确保远程目录存在（远程路径用 path.Dir，始终使用 / 分隔符）
	destDir := path.Dir(dest)
	if err := client.MkdirAll(destDir); err != nil {
		return fmt.Errorf("创建远程目录失败: %w", err)
	}

	destFile, err := client.Create(dest)
	if err != nil {
		return fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer destFile.Close()

	if err := copyWithProgress(ctx, destFile, srcFile, taskID, ch); err != nil {
		// best-effort 删除远程残留文件
		_ = client.Remove(dest)
		return err
	}
	return nil
}

// doDownload 执行下载：远程文件 → 本地
func doDownload(ctx context.Context, src, dest string, client *sftp.Client, taskID int, ch chan<- progressUpdate) error {
	srcFile, err := client.Open(src)
	if err != nil {
		return fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer srcFile.Close()

	// 确保本地目录存在
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0700); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}

	destFile, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer destFile.Close()

	if err := copyWithProgress(ctx, destFile, srcFile, taskID, ch); err != nil {
		// best-effort 删除本地残留文件
		_ = os.Remove(dest)
		return err
	}
	return nil
}

// renderTransferBar 渲染传输进度条（Model 方法）
func (m Model) renderTransferBar() string {
	task := m.transfer.ActiveTask()
	if task == nil {
		return ""
	}

	// 方向箭头
	var arrow string
	if task.Direction == Upload {
		arrow = " ↑ "
	} else {
		arrow = " ↓ "
	}

	// 文件名（截断）
	name := filepath.Base(task.Source)
	if len(name) > 20 {
		name = name[:17] + "..."
	}

	// 进度条
	barWidth := 20
	filled := int(task.Progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := ProgressFilledStyle.Render(strings.Repeat("=", filled)+">") +
		ProgressEmptyStyle.Render(strings.Repeat(" ", barWidth-filled))

	// 大小信息
	sizeInfo := fmt.Sprintf("%s/%s", formatSize(task.Transferred), formatSize(task.Size))

	// 百分比
	pct := fmt.Sprintf("%3d%%", int(task.Progress*100))

	line := fmt.Sprintf("%s%s %s [%s] %s", arrow, name, pct, bar, sizeInfo)
	return StatusBarStyle.Width(m.width).Render(line)
}

// copyWithProgress 带进度回调的复制
func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, taskID int, ch chan<- progressUpdate) error {
	buf := make([]byte, transferBufSize)
	var transferred int64
	lastReport := time.Now()

	for {
		// 检查取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("传输已取消")
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			nw, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("写入失败: %w", writeErr)
			}
			if nw != n {
				return fmt.Errorf("写入不完整: wrote %d of %d bytes", nw, n)
			}
			transferred += int64(nw)

			// 按时间间隔报告进度
			if time.Since(lastReport) >= progressInterval {
				ch <- progressUpdate{
					taskID:      taskID,
					transferred: transferred,
				}
				lastReport = time.Now()
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				// 最后一次进度报告
				ch <- progressUpdate{
					taskID:      taskID,
					transferred: transferred,
				}
				return nil
			}
			return fmt.Errorf("读取失败: %w", readErr)
		}
	}
}
