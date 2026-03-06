# Sprite Sheet 处理流程

将绿幕视频转换为透明背景 sprite sheet，用于 deskpet 项目。

## 输入要求

- 视频格式：MP4，480x640，24fps
- 背景：纯绿色（#00FF00 chroma green）
- 右下角可能有 "HolopixAI" 水印需要去除

## 处理步骤

### 1. 抽帧

用 ffmpeg 以 12fps 抽帧（原始 24fps 减半，减少帧数）：

```bash
ffmpeg -y -i input.mp4 -vf "fps=12" frames/frame_%04d.png
```

### 2. 去除绿色背景

```python
# 检测绿色像素：G通道 > 100，且 G > R*1.3，G > B*1.3
green_mask = (g > 100) & (g > r * 1.3) & (g > b * 1.3)
data[green_mask] = [0, 0, 0, 0]
```

### 3. 去除右下角水印

```python
# 水印区域：底部 12%，右侧 45%
h, w = data.shape[:2]
data[int(h * 0.88):, int(w * 0.55):] = [0, 0, 0, 0]
```

### 4. 清理绿色边缘溢出（green fringe）

```python
avg_rb = (r + b) // 2
greenness = g - avg_rb

# 降低绿色通道
fringe_mask = (a > 0) & (greenness > 30) & (g > 80)
data[:,:,1][fringe_mask] = min(g, avg_rb + 15)

# 强绿边降低透明度
strong_fringe = (a > 0) & (greenness > 50)
data[:,:,3][strong_fringe] = clip(a - greenness, 0, 255)
```

### 5. 自动裁剪

取所有帧的并集 bounding box + 4px padding，统一裁剪尺寸。

### 6. 角色大小归一化

所有动画的角色大小需要和 idle 状态一致（占帧高度约 85%）：

```python
IDLE_RATIO = 0.85
char_ratio = 角色实际高度 / 帧高度  # 从第一帧测量
scale_factor = IDLE_RATIO / char_ratio
```

### 7. 生成 256x256 sprite sheet

- 每帧缩放到 256x256，保持比例，底部对齐（脚部位置一致）
- 排列为 9 列
- 保存为 PNG（RGBA）

```python
TARGET = 256
COLS = 9

s = min(TARGET / fw, TARGET / fh) * scale_factor
# 底部对齐
x_off = (TARGET - nw) // 2
y_off = TARGET - nh
```

## 输出文件

| 文件 | 说明 |
|------|------|
| `asset_new/{name}_spritesheet.png` | 原始尺寸 sprite sheet（备份） |
| `asset_new/{name}_spritesheet_meta.txt` | 元数据（帧数、尺寸、列数、fps） |
| `material/{name}_sheet.png` | 256x256 归一化 sprite sheet（程序使用） |

## 当前素材参数

| 动画 | 帧数 | 原始帧尺寸 | char_ratio | scale_factor |
|------|------|-----------|------------|--------------|
| idle | 121  | 390x554   | 0.978      | 0.869        |
| walk | 73   | 480x622   | 0.871      | 0.976        |
| happy| 73   | 476x566   | 0.958      | 0.887        |
| pick | 73   | 432x560   | 0.968      | 0.878        |
| play | 121  | 454x606   | 0.894      | 0.950        |

## 代码中加载方式

```go
// loadSheetFrames(path, frameW, frameH, cols, totalFrames)
idle = loadSheetFrames("material/idle_sheet.png", 256, 256, 9, 121)
walk = loadSheetFrames("material/walk_sheet.png", 256, 256, 9, 73)
happy = loadSheetFrames("material/happy_sheet.png", 256, 256, 9, 73)
pick = loadSheetFrames("material/pick_sheet.png", 256, 256, 9, 73)
play = loadSheetFrames("material/play_sheet.png", 256, 256, 9, 121)
```
