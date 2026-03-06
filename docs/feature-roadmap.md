# Deskpet 功能演进规划

## Context
当前桌宠已具备基础功能（5种动画状态、圆形图标菜单、鼠标跟随/拖拽）。用户希望综合发展三个方向：情感陪伴、实用工具、趣味互动。本设计规划分4个阶段递进实现。

## 设计决策
- 数据存储：本地 JSON 文件
- 多宠物：多窗口多进程 + IPC
- 角色系统：单角色深度发展（不做皮肤切换）
- 番茄钟 UX：角色旁小字倒计时 + 关键时刻气泡弹窗

---

## Phase 0: 代码重构（基础）

**目标**：拆分 main.go 为模块化包结构，支撑后续功能。

```
deskpet/
  main.go                 -- 入口，组装
  material/               -- 嵌入资源
  internal/
    pet/pet.go            -- Game 主体，Update/Draw/Layout
    pet/state.go          -- 状态机 + 转换表
    anim/anim.go          -- Animation 类型（帧+元数据：loop/oneshot/速度）
    anim/sheet.go          -- sprite sheet 加载器
    ui/menu.go            -- 圆形图标菜单
    ui/bubble.go          -- 气泡对话框（Phase 2）
    ui/timer.go           -- 计时器文字叠加（Phase 2）
    input/mouse.go        -- 点击、拖拽、命中检测
    config/config.go      -- INI 加载
    config/save.go        -- JSON 存档读写
    emotion/emotion.go    -- 情绪系统（Phase 1）
    nurture/nurture.go    -- 养成系统（Phase 3）
    pomodoro/pomodoro.go  -- 番茄钟（Phase 2）
    ipc/ipc.go            -- 多宠物通信（Phase 4）
```

**关键改动**：
- 引入正式状态机（enter/exit hooks + 转换条件表），替代现有 switch
- `Animation` 类型封装帧数据 + loop/oneshot + 播放速度
- 通用 "moveTo(targetX, targetY)" 替代 catchCursor，复用于自主漫步

---

## Phase 1: 自主行为 + 情绪系统

**情绪模型**：
- `Happiness` (0-100)：随时间衰减 -0.5/分钟，互动回升
- `Boredom` (0-100)：idle 无互动时 +1.0/分钟，互动重置为 0

**自主行为**：idle 超过 30-120 秒（随机），按情绪权重选择行为：

| 行为 | 开心时权重 | 无聊时权重 | 动画 |
|------|-----------|-----------|------|
| 漫步 | 30% | 15% | 复用 walk + 随机方向 |
| 伸懒腰 | 25% | 10% | **新：stretch_sheet** |
| 打哈欠 | 20% | 25% | **新：yawn_sheet** |
| 打盹 | 10% | 40% | **新：nap_sheet** |
| 无聊 | 5% | 40% | **新：bored_sheet** |

**存档系统**：`deskpet_save.json`，原子写入（写临时文件 -> rename）
- 触发：退出时、每5分钟、重大状态变化
- 启动时加载，计算离线时间差应用衰减

**新动画需求**：stretch(73f) / yawn(73f) / nap(121f loop) / bored(73f)

---

## Phase 2: 番茄钟 / 提醒

**计时器**：
- 菜单新增第4个按钮（番茄/时钟图标）
- 点击循环：开始专注 -> 取消 -> 跳过休息
- 默认 25分钟专注 / 5分钟短休息 / 15分钟长休息 / 4轮一循环

**显示**：
- 倒计时小字：角色旁边，半透明背景药丸，专注时橙色/休息时绿色
- 气泡弹窗：圆角矩形 + 三角指针，关键时刻弹出（3-5秒后消失）
  - "Let's focus!" / "休息一下!" / "干得漂亮!"

**动画联动**：
- 专注中：可复用 idle（或新增 focused_sheet，nice-to-have）
- 休息提醒：触发 happy/stretch 动画
- 专注模式下禁止自主行为（保持专注动画）

**新增**：菜单图标 `icon_pomodoro.png`，气泡渲染器

---

## Phase 3: 养成系统

**属性**：
- `Hunger` (0-100)：-5/小时衰减，<20 时角色表现悲伤
- `Affection` (0-100)：-2/小时衰减，互动/投喂提升
- `Experience`：累计经验值，决定等级

**喂食**：菜单新增 Feed 按钮，触发 eat 动画，Hunger +30，30分钟冷却

**成长等级**：
- Lv1: 0XP 基础动画
- Lv2: 100XP 解锁 stretch
- Lv3: 300XP 解锁视觉装饰叠加
- Lv4: 600XP 解锁 nap 变体
- Lv5: 1000XP 解锁特殊庆祝动画

**XP 来源**：喂食 +10 / 玩耍 +15 / 完成番茄钟 +25 / 每日登录 +5

**离线处理**：启动时计算 elapsed hours，应用 Hunger/Affection 衰减

**新动画**：eat(73f) / celebration(73f) / 装饰叠加 PNG 2-3个
**新图标**：`icon_feed.png`

---

## Phase 4: 多宠物互动

**架构**：多进程，Unix socket IPC
- 第一个启动的进程为 host（创建 socket server）
- 后续进程为 guest（连接 socket）
- Host 退出时 guest 自动晋升

**协议**：JSON over Unix socket
- `position`：每 200ms 广播窗口坐标
- `state`：状态变化时广播
- `interact`：距离 <300px 时触发社交行为

**社交行为**：两只宠物靠近时随机触发
- 互相走近 -> 同步播放 play 动画 -> 分开

**Socket 路径**：`$TMPDIR/deskpet.sock` (macOS) / `\\.\pipe\deskpet` (Windows)

---

## 全部新动画资产汇总

| Phase | 动画 | 类型 | 帧数 | 优先级 |
|-------|------|------|------|--------|
| 1 | stretch | oneshot | ~73 | 必须 |
| 1 | yawn | oneshot | ~73 | 必须 |
| 1 | nap | loop | ~121 | 必须 |
| 1 | bored | oneshot | ~73 | 必须 |
| 2 | focused | loop | ~121 | 可复用 idle |
| 3 | eat | oneshot | ~73 | 必须 |
| 3 | celebration | oneshot | ~73 | 必须 |

所有动画走现有 pipeline：视频(480x640,24fps,绿幕) -> 12fps抽帧 -> 去背景 -> 256x256归一化 -> 9列sprite sheet

## 数据模型

```go
type SaveData struct {
    Version  int          `json:"version"`
    PetID    string       `json:"pet_id"`
    Emotion  EmotionData  `json:"emotion"`
    Nurture  NurtureData  `json:"nurture"`
    Pomodoro PomodoroData `json:"pomodoro"`
    Position PositionData `json:"position"`
}

type EmotionData struct {
    Happiness       float64 `json:"happiness"`
    Boredom         float64 `json:"boredom"`
    LastInteraction string  `json:"last_interaction"` // RFC3339
}

type NurtureData struct {
    Hunger         float64  `json:"hunger"`
    Affection      float64  `json:"affection"`
    Experience     int      `json:"experience"`
    Level          int      `json:"level"`
    LastFeedTime   string   `json:"last_feed_time"`
    LastUpdateTime string   `json:"last_update_time"`
    Unlocks        []string `json:"unlocks"`
}

type PomodoroData struct {
    TotalSessions   int    `json:"total_sessions"`
    TodaySessions   int    `json:"today_sessions"`
    LastSessionDate string `json:"last_session_date"`
}

type PositionData struct {
    X int `json:"x"`
    Y int `json:"y"`
}
```

## 验证方式
- 每个 Phase 完成后 `go run .` 验证交互
- Phase 0：确认所有现有功能不变
- Phase 1：观察 idle 30秒后是否触发自主行为，退出重启后情绪值是否持久化
- Phase 2：菜单启动番茄钟，验证倒计时显示和气泡弹窗
- Phase 3：喂食、查看经验增长、等级提升解锁新动画
- Phase 4：启动两个进程，观察宠物靠近时互动
