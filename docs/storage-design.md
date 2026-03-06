# 番茄钟数据持久化 - 设计文档

## Context

deskpet 当前所有运行时数据在退出后丢失。使用 BoltDB (bbolt) 持久化番茄钟会话历史，支持详细记录和按日/周/月查询。

## 决策摘要

| 项目 | 决定 |
|------|------|
| 存储引擎 | go.etcd.io/bbolt（纯 Go，无 CGO） |
| 存储范围 | 番茄钟会话历史 |
| 数据粒度 | 每个阶段独立记录（含开始/结束时间、是否完成） |
| 文件位置 | 程序同目录 `deskpet.db`（运行时自动创建，不随二进制发布） |

## 数据结构

```go
type Session struct {
    StartTime time.Time     `json:"start_time"`
    EndTime   time.Time     `json:"end_time"`
    Duration  time.Duration `json:"duration"`    // 计划时长
    PhaseType WorkState     `json:"phase_type"`  // WorkFocus / WorkShortBreak / WorkLongBreak
    Completed bool          `json:"completed"`   // true=自然结束, false=手动中断
}

type StatsResult struct {
    TotalSessions int
    TotalFocusMin int
    TodaySessions int
    TodayFocusMin int
    WeekSessions  int
    WeekFocusMin  int
    MonthSessions int
    MonthFocusMin int
}
```

## BoltDB Bucket 设计

- **`sessions`** bucket：存储每次会话记录
  - Key: `YYYYMMDD-HHmmss-NNNNNNNNN`（日期前缀，支持 `Cursor.Seek` 范围查询）
  - Value: JSON 编码的 `Session`
- **`meta`** bucket：存储 schema 版本
  - Key: `schema_version` → Value: `"1"`

日期前缀 key 设计使按日查询只需 `Seek("20260306")` + 前缀迭代。

## 文件变更

### 1. 新建 `storage.go`

Storage 层，包含：

```go
type Storage struct { db *bbolt.DB }

func OpenStorage(path string) (*Storage, error)  // 打开DB，创建bucket，检查schema版本
func (s *Storage) Close() error
func (s *Storage) SaveSession(sess Session) error
func (s *Storage) QuerySessionsByDate(prefix string) ([]Session, error)  // prefix: "20260306"
func (s *Storage) QuerySessionsRange(startDate, endDate string) ([]Session, error)
func (s *Storage) ComputeStats() (StatsResult, error)
```

OpenStorage 中创建 bucket 并设置 schema_version，为未来迁移预留。

### 2. 修改 `main.go`

**A. deskpet 结构体添加字段**（约 L212-223）

```go
storage       *Storage    // BoltDB 存储
workStartTime time.Time   // 当前阶段开始时间
```

**B. 记录阶段开始时间** — 在三个阶段启动方法中加 `d.workStartTime = time.Now()`：
- `startFocusPhase()` (L295)
- `startShortBreak()` (L307)
- `startLongBreak()` (L320)

**C. 添加 `saveCurrentPhase` 辅助方法**

```go
func (d *deskpet) saveCurrentPhase(completed bool) {
    if d.storage == nil || d.workState == WorkIdle { return }
    sess := Session{
        StartTime: d.workStartTime,
        EndTime:   time.Now(),
        Duration:  d.workTotalDuration,
        PhaseType: d.workState,
        Completed: completed,
    }
    if err := d.storage.SaveSession(sess); err != nil {
        log.Printf("failed to save session: %v", err)
    }
}
```

**D. 阶段自然结束时保存** — `updateWorkMode()` (L342-363)：在 switch 之前调用 `d.saveCurrentPhase(true)`

**E. 手动停止时保存** — `stopWorkMode()` (L333)：在重置状态之前调用 `d.saveCurrentPhase(false)`

**F. main() 中打开/关闭 DB**（L1040-1106）：
- `loadConfig` 之后调用 `OpenStorage("deskpet.db")`，失败不致命（log warning，继续无持久化运行）
- pet 初始化时赋值 `storage: store`
- `ebiten.RunGameWithOptions` 返回后调用 `store.Close()`

### 3. 修改 `go.mod`

添加 `go.etcd.io/bbolt` 依赖。

## 线程安全

Ebiten 的 `Update()` 在单 goroutine 运行，所有 session 保存都在 `Update()` 中触发，无并发问题。BoltDB 的读事务 (`db.View`) 本身支持并发。

## 边界情况

- **多实例运行**：BoltDB 文件锁，第二个实例打开失败 → 降级为无持久化运行
- **强制杀死进程**：当前阶段未保存丢失，可接受
- **time.Duration JSON**：序列化为纳秒 int64，存取一致无问题

## 验证方式

1. `go build` 编译通过
2. 启动 deskpet，进入工作模式，完成一个专注阶段 → 检查 `deskpet.db` 文件已创建
3. 手动中断一个阶段 → 检查记录的 `completed=false`
4. 退出后重启 → 用临时测试代码或未来 UI 验证历史数据仍在
5. 删除 `deskpet.db` 启动 → 自动创建新数据库，无报错
