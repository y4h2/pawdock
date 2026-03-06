# 工作模式设计文档（番茄钟 + 喝水提醒）

## 核心概念

菜单新增"工作模式"按钮，一键进入，自动管理专注/休息/喝水循环。整合番茄钟计时与喝水提醒为统一的工作流。

## 工作节奏

经典番茄钟循环：
- 25分钟专注 -> 5分钟短休息(提醒喝水) -> 循环4轮 -> 15分钟长休息
- 每次休息时自动提醒喝水
- 时间可在 deskpet.ini 配置

## 状态流转

```
点击"工作" -> FocusWork(25min) -> BreakRemind(5min,喝水) -> FocusWork -> ... -> LongBreak(15min) -> 回到Idle
                                                                       (循环4轮)
菜单可随时取消工作模式 -> 回到Idle
```

## 角色行为变化

### 专注中
- 停止跟随鼠标，角色安静不动（复用 idle 或新增 focused_sheet）
- 禁止自主行为（不漫步/打哈欠等）
- 进入专注时气泡弹出

### 休息提醒
- 角色播放 happy/stretch 动画，活泼起来
- 气泡提醒喝水/休息
- 恢复鼠标跟随（休息时可以玩）

### 长休息
- 角色播放 play 动画
- 气泡庆祝完成一轮

### 完成/取消
- 角色播放 happy 动画
- 气泡告别

## 倒计时显示：右侧圆形徽章

- 角色右侧绘制一个小圆形徽章（直径约 24px）
- 圆形外圈作为进度环（弧形），显示剩余时间比例
- 圆内显示分钟数 "23"（不显示秒，保持简洁）
- 专注时：暖橙色进度环
- 休息时：清新绿色进度环
- 仅在工作模式激活时显示

### 技术实现
- 使用 `vector.DrawFilledCircle` 画底色圆
- 使用 `vector.StrokeLine` 或路径画弧形进度环
- 使用 `bitmapfont` 在圆内绘制分钟数
- 位置：角色中心右侧偏上，与菜单按钮不重叠

## 气泡对话系统

### 样式
- 圆角矩形 + 底部三角指针指向角色
- 出现在角色头顶偏右
- 3-5秒后自动消失（淡出）
- 半透明深色背景 + 白色文字

### 台词库（可爱口语化，每次随机选一句）

**专注开始**：
- "开始专注啦，加油~"
- "一起努力吧！"
- "专注模式启动~"

**休息提醒**：
- "辛苦了，休息一下吧！"
- "记得喝水哦~"
- "站起来活动活动~"
- "眼睛也要休息一下哦！"

**长休息**：
- "太棒了！完成一轮啦~"
- "好厉害！休息久一点吧！"

**完成/取消**：
- "今天也很努力呢！"
- "干得漂亮！"

### 技术实现
```go
type Bubble struct {
    Text       string
    Remaining  int     // 剩余 ticks
    Alpha      float64 // 透明度，用于淡出
}

func (b *Bubble) Show(text string, durationSec int) {
    b.Text = text
    b.Remaining = durationSec * 50 // TPS=50
    b.Alpha = 1.0
}

func (b *Bubble) Update() {
    if b.Remaining <= 0 { return }
    b.Remaining--
    // 最后1秒淡出
    if b.Remaining < 50 {
        b.Alpha = float64(b.Remaining) / 50.0
    }
}
```

## 声音提示

- 专注结束时播放一声轻柔提示音
- 使用 ebiten/audio 包播放嵌入的 ogg 文件
- 音效文件嵌入到 material/ 中（<50KB）
- 仅在状态切换时播放一次

## 菜单交互

- 新增第4个圆形按钮：工作模式图标
- 非工作模式时：显示时钟/番茄图标，点击 -> 开始工作模式
- 工作模式中：图标切换为"停止"样式，点击 -> 结束工作模式
- 暖色系配色延续现有菜单风格

## 数据持久化

```go
type PomodoroData struct {
    TotalSessions   int    `json:"total_sessions"`    // 累计完成的专注次数
    TodaySessions   int    `json:"today_sessions"`    // 今日完成次数
    LastSessionDate string `json:"last_session_date"` // 上次记录日期，用于重置今日计数
    TotalFocusMin   int    `json:"total_focus_minutes"` // 累计专注分钟数
}
```

保存在 `deskpet_save.json` 中，与情绪/养成数据一起管理。

## 配置项

在 deskpet.ini 中新增：
```ini
FocusDuration = 25      # 专注时长（分钟）
ShortBreak = 5          # 短休息（分钟）
LongBreak = 15          # 长休息（分钟）
CyclesBeforeLong = 4    # 几轮后长休息
```

## 新增资源

| 资源 | 说明 | 优先级 |
|------|------|--------|
| icon_work.png | 工作模式菜单图标 | 必须 |
| icon_stop.png | 停止工作模式图标 | 必须 |
| notify.ogg | 提示音效 (<50KB) | 必须 |
| focused_sheet.png | 专注时角色动画 | nice-to-have，先复用 idle |

## 实现步骤

1. 添加番茄钟状态机逻辑（FocusWork / ShortBreak / LongBreak 状态）
2. 实现气泡对话渲染器（bubble.go）
3. 实现圆形徽章倒计时显示（timer badge）
4. 菜单新增工作模式按钮 + 图标
5. 集成音效播放（ebiten/audio）
6. 连接角色行为（专注时停止跟随，休息时恢复）
7. 添加配置项解析
8. 数据持久化

## 验证方式

- `go run .` 启动后点击工作模式按钮
- 验证倒计时徽章显示和进度环动画
- 验证专注结束时气泡弹出 + 音效播放
- 验证专注中角色不跟随鼠标
- 验证休息时角色恢复跟随 + 播放动画
- 验证取消工作模式回到正常状态
- 验证配置项修改生效
