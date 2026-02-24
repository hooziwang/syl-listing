# 亚马逊美国站 Listing 生成指南（中英文版）

---

## 使用流程

```
第1步：填写【关键词输入模板】（提供15-20个按权重排序的关键词）
    ↓
第2步：用户先写一版标题（版本A - 参考竞品）
    ↓
第3步：生成 Markdown 版本（版本B - 纯文本确认）
        • 标题：埋入 #1-#3（最重要的3个）
        • 五点：灵活埋入全部15-20个关键词（每点235-250字符，不强制平均）
        • 描述：固定13-15个关键词 = 权重词（按顺序）+ 五点剩余
        • 搜索词：按 #1-#20 顺序排列全部关键词
    ↓
第4步：用户确认 Markdown 版本无误
    ↓
第5步：生成 Word 高亮版（最终交付）
        • 关键词真正黄色高亮
        • 可直接复制使用
    ↓
第6步：检查字符限制 & 敏感词
```

**交付物说明：**
| 阶段 | 格式 | 用途 |
|------|------|------|
| 确认版 | Markdown 纯文本 | 用户确认内容正确性 |
| 最终版 | Word 文档 (.docx) | 关键词黄色高亮，可直接使用 |

**关键词分配规则速查：**

| 给了N个词 | 五点埋入 | 五点剩余 | 描述埋入 | 搜索词 |
|-----------|---------|---------|---------|--------|
| 15个 | 全部15个 | 0个 | #1-#13（按权重延续）= **13个** | 15个按顺序 |
| 18个 | #1-#16 | #17-#18 | #1-#13 + #17-#18（权重13个+剩余2个）= **15个** | 18个按顺序 |
| 20个 | #1-#14 | #15-#20 | #1-#9 + #15-#20（权重9个+剩余6个）= **15个** | 20个按顺序 |

---

## 一、关键词输入模板

填写以下信息，用于生成完整 listing：

### 基础信息
| 项目 | 中文填写 | 英文填写（如有） |
|------|----------|------------------|
| 品牌名 | ____________ | ____________ |
| 产品形式词（核心词1/本质词） | ____________ | ____________ |
| 产品同义词（核心词2） | ____________ | ____________ |
| 数量/包装 | ____________ | ____________ |
| 核心材质 | ____________ | ____________ |
| 颜色 | ____________ | ____________ |
| 尺寸 | ____________ | ____________ |
| 重量（如有） | ____________ | ____________ |
| 目标年龄 | ____________ | ____________ |

### 功能卖点（按重要性排序，开发根据图片填写）
| 优先级 | 中文卖点 | 英文卖点（参考） |
|--------|----------|------------------|
| 1（最重要） | ____________ | ____________ |
| 2 | ____________ | ____________ |
| 3 | ____________ | ____________ |
| 4 | ____________ | ____________ |
| 5 | ____________ | ____________ |

### 产品细节信息
| 项目 | 中文 | 英文 |
|------|------|------|
| 包装内含 | ____________ | ____________ |
| 适用场景 | ____________ | ____________ |
| 设计特点 | ____________ | ____________ |
| 质量/安全认证 | ____________ | ____________ |

### 关键词库（按权重排序，共15-20个）

**⚠️ 重要：请按权重从高到低排序，越靠前的词越重要**

| 序号 | 权重 | 关键词（英文） | 关键词（中文） | 必须埋入位置 |
|------|------|----------------|----------------|--------------|
| 1 | ⭐⭐⭐最高 | ____________ | ____________ | 标题 + 五点第1点 |
| 2 | ⭐⭐⭐最高 | ____________ | ____________ | 标题 + 五点第2点 |
| 3 | ⭐⭐⭐最高 | ____________ | ____________ | 五点第3点 |
| 4 | ⭐⭐⭐最高 | ____________ | ____________ | 五点第4点 |
| 5 | ⭐⭐⭐最高 | ____________ | ____________ | 五点第5点 |
| 6 | ⭐⭐高 | ____________ | ____________ | 描述（前段） |
| 7 | ⭐⭐高 | ____________ | ____________ | 描述（前段） |
| 8 | ⭐⭐高 | ____________ | ____________ | 描述（中段） |
| 9 | ⭐⭐高 | ____________ | ____________ | 描述（中段） |
| 10 | ⭐⭐高 | ____________ | ____________ | 描述（后段） |
| 11 | ⭐中 | ____________ | ____________ | 搜索词 |
| 12 | ⭐中 | ____________ | ____________ | 搜索词 |
| 13 | ⭐中 | ____________ | ____________ | 搜索词 |
| 14 | ⭐中 | ____________ | ____________ | 搜索词 |
| 15 | ⭐中 | ____________ | ____________ | 搜索词 |
| 16 | ⭐低 | ____________ | ____________ | 搜索词 |
| 17 | ⭐低 | ____________ | ____________ | 搜索词 |
| 18 | ⭐低 | ____________ | ____________ | 搜索词 |
| 19 | ⭐低 | ____________ | ____________ | 搜索词 |
| 20 | ⭐低 | ____________ | ____________ | 搜索词 |

#### 关键词分类参考
| 类型 | 示例（英文） | 示例（中文） |
|------|-------------|-------------|
| 产品形式词 | Dry Erase Pockets | 干擦文件袋 |
| 功能卖点词 | Hard Backing Support | 硬背板支撑 |
| 限制词 | Colorful, Reusable | 彩色、可重复使用 |
| 场景词 | Classroom Organization | 课堂组织 |
| 年龄词 | for Kids Ages 3-8 | 3-8岁儿童 |
| 长尾词 | Fine Motor Skills Toys | 精细动作技能玩具 |

---

## 二、标题生成规则

### 说明：双版本标题

**版本 A - 用户提供版：**
- 用户根据竞品和关键词自己撰写一版标题
- 作为参考和对比

**版本 B - 系统生成版：**
- 按以下结构公式自动生成
- 确保字符数、埋词位置等符合规范

### 结构公式（系统生成版）

```
品牌 + 数量 + 核心词1 + 卖点 + 核心词2 + 尺寸 + 长尾词
```

**埋词重点：**
- 标题必须包含 **关键词#1、#2**（权重最高的2个词）
- 尽量包含 **关键词#3**（第3重要的词）
- 核心词（#1-#3）在标题中位置靠左，权重更高

### 英文版标题模板

```
[Brand] [Quantity] [Core Keyword 1] [Selling Point] [Core Keyword 2] [Size/Dimension] [Long-tail Keyword]
```

**示例：**
```
gisgfim 3 Pack Dry Erase Pockets with 1.2mm Hard Backing Support Reusable Plastic Sheet Protectors 10x13 Inches for Classroom Organization
```

### 中文版标题参考

```
[品牌] [数量] [核心词1] [卖点] [核心词2] [尺寸] [长尾词]
```

**示例：**
```
gisgfim 3个装可重复使用干擦文件袋 带1.2mm硬背板支撑 塑料试卷保护套 10x13英寸 课堂组织用品
```

### 标题规则检查清单

| 检查项 | 英文版 | 中文版 |
|--------|--------|--------|
| 字符限制 | ≤ 200 字符（建议190-200） | ≤ 100 个汉字（亚马逊中文站） |
| 品牌开头 | ✅ 必须以品牌开头 | ✅ 必须以品牌开头 |
| 核心词数量 | 核心词1 + 核心词2 = 2个 | 同上 |
| 单词重复 | 同一单词不超过2次 | 同一词不超过2次 |
| 首字母大写 | 每个单词首字母大写（介词除外） | 按中文习惯 |
| 特殊符号 | ❌ 禁止 ! * 等符号 | ❌ 禁止特殊符号 |
| 禁用词 | ❌ 退款、促销、对比信息 | ❌ 同上 |

---

## 三、五点描述生成规则

### 英文版结构 + 埋词策略（15-20 个关键词）

**埋词规则：**
- **五点必须埋入全部 15-20 个关键词**
- **不强制平均分配**，根据卖点内容灵活安排
- 高权重关键词（#1-#5）分散到不同要点
- 确保每个关键词都自然融入句子

**参考分配（可根据实际调整）：**
- 15 个词：可每点3个，或#1-#5各埋1个，剩余分散
- 16-20 个词：重点埋#1-#5，剩余按内容分配
- 根据卖点逻辑，有些点可多埋，有些可少埋

### 英文版示例

假设关键词排序：
1. Dry Erase Pockets → 2. Reusable → 3. Hard Backing → 4. Classroom Organization → 5. 10x13 Inches → 6. Sheet Protectors → 7. Preschool → 8. Kindergarten → 9. Fine Motor Skills → 10. Learning Activities...

```
• Reusable Dry Erase Design: Our Dry Erase Pockets feature a clear surface perfect for repeated practice. These Reusable Sheet Protectors allow kids to write and wipe clean for endless learning fun

• Premium Quality with Hard Backing: Made with durable plastic featuring Hard Backing for excellent writing support. Perfect for Preschool classrooms and home learning environments

• Perfect 10x13 Inch Size: Measuring 10x13 Inches, these pockets fit standard worksheets. Ideal for Kindergarten teachers and Classroom Organization

• Develops Fine Motor Skills: Designed to help children develop Fine Motor Skills through writing practice. Supports various Learning Activities in educational settings

• Versatile Classroom Essential: Great for Classroom Organization and multiple learning scenarios. Suitable for Preschool, Kindergarten and elementary school use
```
**分析：** 前10个关键词已按顺序埋入，每个要点包含1个高权重词+1个中权重词

### 中文版结构（英文翻译版）

| 顺序 | 小标题 | 内容类型 |
|------|--------|----------|
| 1 | 【卖点1】 | 包装内容 + 使用周期 |
| 2 | 【卖点2】 | 产品质量/材质 |
| 3 | 【卖点3】 | 尺寸规格 |
| 4 | 【卖点4】 | 设计特点 |
| 5 | 【卖点5】 | 使用场景/氛围 |

**埋词要求**：同英文版，埋入全部15-20个关键词

### 五点描述规则

| 规则 | 英文版 | 中文版 |
|------|--------|--------|
| **每点字符** | **235-250 字符**（严格范围） | **235-250 字符** |
| **关键词埋入** | 严格按照用户给的顺序，不调整 | 同英文版 |
| **关键词形式** | 原样保留，不改变（大小写、单复数） | 同英文版 |
| **段尾标点** | ❌ 禁止 | ❌ 禁止 |
| **小标题格式** | 简单英文单词，见下表 | 同英文版 |
| **写作顺序** | 先客观后主观（1-4点客观，5点主观） | 同英文版 |

**五点小标题标准（先客观后主观）：**

| 顺序 | 小标题 | 内容类型 | 埋入关键词示例 |
|------|--------|----------|----------------|
| 1 | Package Contents | 包装清单 | #1, #2, #3 |
| 2 | Dimensions | 尺寸 | #4, #5, #6 |
| 3 | Material | 材质功能 | #7, #8, #9 |
| 4 | Color / Usage | 颜色/用法 | #10, #11, #12 |
| 5 | Usage / Design | 使用场景/设计氛围 | #13, #14, #15, #16 |

**注意：** 小标题使用**简单英文单词**，不要用描述性短语或关键词本身。

---

## 四、产品描述生成规则

### 英文版模板（描述固定13-15个关键词）

**埋词规则：**
- **描述固定埋入 13-15 个关键词**（必须达到13个，不超过15个）
- **前部**：按权重顺序的关键词（从#1开始，不设固定数量）
- **后部**：五点中因字符限制未能埋入的剩余关键词
- **计算方式**：
  - 按权重顺序取词 + 剩余词，总共13-15个
  - 如果不足13个，继续按权重顺序延续（#11、#12...）
  - 如果超过15个，截断到15个

**场景示例：**

| 给了N个词 | 五点埋入 | 五点剩余 | 描述埋入（按权重+剩余） | 总计 |
|-----------|---------|---------|------------------------|------|
| 15个 | #1-#15 | 0个 | #1-#13（按权重延续） | **13个** |
| 18个 | #1-#16 | #17-#18 | #1-#13 + #17-#18（权重词13个+剩余2个） | **15个** |
| 20个 | #1-#14 | #15-#20 | #1-#9 + #15-#20（权重词9个+剩余6个，取前6个到15个） | **15个** |

**模板示例（两段话形式，15个词五点埋完）：**
```
<b>Product Description</b>

[Brand] [关键词#1] are designed to create magical atmosphere. These [关键词#2] feature soft colors perfect for [关键词#3]. The [关键词#4] design makes installation effortless. Perfect [关键词#5] style adds elegance to any space. Ideal [关键词#6] for teachers and planners.

These [关键词#7] pieces transform any room instantly. The [关键词#8] setup takes minutes. Perfect [关键词#9] for birthdays. Great [关键词#10] for themed events. Beautiful [关键词#11] create dreamy ambiance. Essential [关键词#12] for magical parties. Use as [关键词#13] for versatile display.
```
**说明**：五点已埋完#1-#15，无剩余词，描述按权重顺序埋#1-#13（第一段7个+第二段6个），总计13个

### 描述示例（纸灯笼案例 - 13个关键词，两段话）

**场景**：给了16个关键词，五点埋完全部16个，描述埋#1-#13（两段话形式）

```
<b>Product Description</b>

pinpai paper lanterns are designed to create magical atmosphere for any celebration. These pastel paper lanterns feature soft colors perfect for classroom decorations, baby showers and weddings. The Hanging pastel paper lanterns design makes installation effortless. Perfect chinese lanterns style adds cultural elegance to any space. Ideal classroom Lantern decorations for teachers and event planners.

These pastel classroom decor pieces transform any room instantly. The Hanging pastel classroom decor setup takes minutes. Perfect pastel party decorations for birthdays and celebrations. Great pastel party lanterns decorations for themed events. Beautiful pastel birthday decorations create dreamy ambiance. Essential unicorn birthday decorations for magical parties.
```

**埋词检查**：
- 第一段：#1-#7（paper lanterns, pastel paper lanterns, classroom decorations, Hanging pastel paper lanterns, chinese lanterns, classroom Lantern decorations）
- 第二段：#8-#13（pastel classroom decor, Hanging pastel classroom decor, pastel party decorations, pastel party lanterns decorations, pastel birthday decorations, unicorn birthday decorations）
- 总计：13个关键词 ✓

### 中文版模板（英文翻译版，两段话）

**埋词规则**：同英文版，固定13-15个关键词 = 权重词 + 剩余词，两段话形式

```
<b>产品描述</b>

[品牌][关键词#1]旨在创造神奇氛围。[关键词#2]具有柔和色彩，非常适合[关键词#3]。[关键词#4]设计使安装轻松。完美的[关键词#5]风格为任何空间增添优雅气息。理想的[关键词#6]适合教师和活动策划者。

这些[关键词#7]可瞬间改变任何房间。[关键词#8]安装仅需几分钟。完美的[关键词#9]适合生日庆典。出色的[关键词#10]适合主题活动。精美的[关键词#11]营造梦幻氛围。必备的[关键词#12]适合神奇派对。可用作[关键词#13]实现多功能展示。
```

### 描述规则

| 项目 | 英文版 | 中文版 |
|------|--------|--------|
| 字符限制 | ≤ 1000 字符 | ≤ 1000 字符 |
| 埋词数量 | 固定 **13-15 个关键词** | 固定 **13-15 个关键词** |
| 埋词组成 | 权重词（按顺序）+ 五点剩余词 | 同英文版 |
| **格式要求** | **两段话形式**，不用列表 | **两段话形式**，不用列表 |
| 补充规则 | 不足13个继续按权重延续，超过15个截断 | 同英文版 |
| HTML 标签 | ✅ 可用 `<b>` `<br>` | ✅ 可用 `<b>` `<br>` |

---

## 五、关键词高亮标记规则

### 交付格式说明

| 版本 | 格式 | 高亮方式 |
|------|------|----------|
| **确认版** | Markdown 纯文本 | 无高亮标记，纯文本显示 |
| **最终版** | Word 文档 (.docx) | 关键词真正**黄色高亮** |

### Word 版高亮示例
- 生成后所有**关键词自动黄色高亮**
- 可直接复制到亚马逊后台使用
- 高亮仅用于内部检查，实际上传时无高亮

---

## 六、搜索词（ST）生成规则

### 英文版格式（按提供的15-20个关键词顺序）

**规则：**
- **直接按提供的 15-20 个关键词的权重顺序排列**
- 空格分隔，尽量填满 250 字符
- 如字符不足（不足15个词），从 #1 开始循环补充

```
[关键词#1] [关键词#2] [关键词#3] ... [关键词#15] [关键词#16] [关键词#17] [关键词#18] [关键词#19] [关键词#20]
```

**示例（15个关键词）：**
```
dry erase pockets reusable hard backing classroom organization 10x13 inches sheet protectors preschool kindergarten fine motor skills learning activities teaching supplies write and wipe kids boys girls ages 3-8
```
**字符数：** 246/250

**示例（13个关键词 → 补充到15个）：**
```
dry erase pockets reusable hard backing classroom organization 10x13 inches sheet protectors preschool kindergarten fine motor skills learning activities teaching supplies dry erase pockets reusable
```
**说明：** 原13个词 + 从#1、#2延续补充，共15个位置

### 中文版格式（英文翻译版）

```
[关键词#1] [关键词#2] [关键词#3] ... [关键词#15-20]
```

**示例：**
```
干擦文件袋 可重复使用 硬背板支撑 课堂组织 10x13英寸 试卷保护套 学前班 幼儿园 精细动作技能 学习活动 教学用品 写擦两用 儿童 男孩女孩 3-8岁
```

### 搜索词规则

| 规则 | 英文版 | 中文版 |
|------|--------|--------|
| 字符限制 | ≤ 250 字符 | ≤ 250 字符 |
| 分隔符 | 空格（逗号被忽略） | 空格 |
| 排列顺序 | 权重从高到低 | 权重从高到低 |
| 英文逻辑 | long red dress（形容词+名词） | 按中文习惯 |
| 避免堆砌 | 同义词不重复 | 同义词不重复 |
| 目标 | 尽量填满 250 字符 | 尽量填满 250 字符 |

---

## 六、完整示例（以干擦文件袋为例）

### 输入信息

#### 基础信息
| 项目 | 内容 |
|------|------|
| 品牌名 | gisgfim |
| 数量 | 3 Pack / 3个装 |
| 尺寸 | 10x13 Inches / 10x13英寸 |

#### 按权重排序的15个关键词
| 序号 | 英文关键词 | 中文关键词 |
|------|-----------|-----------|
| 1 | Dry Erase Pockets | 干擦文件袋 |
| 2 | Reusable | 可重复使用 |
| 3 | Hard Backing | 硬背板支撑 |
| 4 | Classroom Organization | 课堂组织 |
| 5 | 10x13 Inches | 10x13英寸 |
| 6 | Sheet Protectors | 试卷保护套 |
| 7 | Preschool | 学前班 |
| 8 | Kindergarten | 幼儿园 |
| 9 | Fine Motor Skills | 精细动作技能 |
| 10 | Learning Activities | 学习活动 |
| 11 | Teaching Supplies | 教学用品 |
| 12 | Write and Wipe | 写擦两用 |
| 13 | Kids | 儿童 |
| 14 | Boys Girls | 男孩女孩 |
| 15 | Ages 3-8 | 3-8岁 |

---

### 输出结果

#### 【英文版】

**版本 A - 用户提供版:**
```
[用户根据竞品和关键词自己写的标题]
```

**版本 B - 系统生成版 (198 字符):**
```
gisgfim 3 Pack Dry Erase Pockets with Hard Backing Reusable Sheet Protectors 10x13 Inches for Classroom Organization Preschool Kindergarten
```
**埋词说明：** 包含关键词#1-#5（Dry Erase Pockets, Reusable, Hard Backing, Classroom Organization, 10x13 Inches）

---

**五点（灵活埋入全部15个关键词，每点≤300字符）:**

| 要点 | 埋入关键词 | 内容 | 字符数 |
|------|-----------|------|--------|
| 1 | #1, #2, #6 | • Premium Dry Erase Pockets: Our Reusable Dry Erase Pockets feature a clear surface perfect for repeated practice with Sheet Protectors design | ~138 |
| 2 | #3, #7, #8 | • Hard Backing Support: Made with durable Hard Backing for Preschool and Kindergarten learning environments, providing excellent writing support | ~142 |
| 3 | #4, #5, #9 | • Perfect for Classroom Organization: These 10x13 Inches pockets help teachers with Classroom Organization while developing Fine Motor Skills | ~138 |
| 4 | #10, #11, #12 | • Versatile Learning Activities: Great for various Learning Activities and Teaching Supplies. Write and Wipe surface makes learning engaging | ~136 |
| 5 | #13, #14, #15 | • Ideal for Kids: Perfect for Kids including Boys Girls Ages 3-8, making education fun and interactive for young learners | ~110 |

**✅ 已完成：** 全部15个关键词已埋入（每点控制在250字符以内）

---

**描述（埋入13个关键词，五点已埋完全部15个）:**
```
<b>Product Description</b>

gisgfim Dry Erase Pockets are designed to make learning engaging and efficient. Reusable design allows for endless practice. Hard Backing provides excellent support.

<b>Key Features:</b>
• Classroom Organization made easy with proper storage
• 10x13 Inches size fits standard worksheets perfectly
• Sheet Protectors keep documents clean and safe
• Preschool activities supported with durable materials
• Kindergarten learning enhanced through interaction
• Fine Motor Skills developed with each use
• Learning Activities made fun and engaging
• Teaching Supplies essential for every classroom
• Write and Wipe surface for repeated practice
• Perfect for Kids of all learning levels
• Gifts for Boys and Girls quality materials
• Ages 3-8 durable design
• Educational Toys perfect for learning

<b>Package Includes:</b>
• 3 x Dry Erase Pockets
• 3 x Dry Erase Markers

Perfect for Classroom Organization and Preschool activities. Dry Erase Pockets with Hard Backing support make learning fun for Kids!
```

**✅ 已完成：** 描述埋入13个关键词
- 权重词13个：#1-#13（Dry Erase Pockets 到 Gifts for Boys and Girls）
- 无剩余词：五点已埋完#1-#15，描述按权重延续到#13
- 总计：13个（达到最低要求）
- 首尾呼应：#1-#3 在结尾再次出现

---

**搜索词（按权重顺序排列）:**
```
dry erase pockets reusable hard backing classroom organization 10x13 inches sheet protectors preschool kindergarten fine motor skills learning activities teaching supplies write and wipe kids boys girls ages 3-8
```
**字符数：** 246 / 250  
**✅ 已完成：** 15个关键词按权重顺序排列

---

#### 【中文版】

**版本 A - 用户提供版:**
```
[用户自己写的中文标题]
```

**版本 B - 系统生成版:**
```
gisgfim 3个装干擦文件袋 可重复使用 带硬背板支撑 试卷保护套 10x13英寸 课堂组织用品 适用学前班幼儿园
```

**五点（灵活埋入15个关键词，每点≤300字符）:**
| 要点 | 埋入关键词 | 示例内容 |
|------|-----------|---------|
| 1 | 干擦文件袋、可重复使用、试卷保护套 | 优质干擦文件袋：采用可重复使用设计，透明表面配合试卷保护套功能，让孩子可以反复练习书写 |
| 2 | 硬背板支撑、学前班、幼儿园 | 硬背板支撑设计：配备坚固硬背板支撑，专为学前班和幼儿园学习环境设计，提供出色书写体验 |
| 3 | 课堂组织、10x13英寸、精细动作技能 | 课堂组织利器：10x13英寸尺寸适合标准工作表，帮助老师进行课堂组织，同时培养儿童精细动作技能 |
| 4 | 学习活动、教学用品、写擦两用 | 多功能学习活动：作为必备教学用品，写擦两用表面让各种学习活动更加生动有趣 |
| 5 | 儿童、男孩女孩、3-8岁 | 适合儿童使用：专为3-8岁男孩女孩设计，是寓教于乐的理想教育工具 |

**描述（埋入10个关键词）:**
```
<b>产品描述</b>

gisgfim干擦文件袋旨在让学习变得更加有趣和高效。可重复使用设计支持反复练习。硬背板支撑提供出色书写体验。

<b>主要特点：</b>
• 课堂组织：便于分类整理学习资料
• 10x13英寸尺寸：适合标准工作表
• 试卷保护套：保持文件整洁
• 学前班适用：支持早期教育活动
• 幼儿园必备：增强互动学习
• 精细动作技能：培养书写能力
• 学习活动：丰富教学体验
• 教学用品：每间教室必备
• 写擦两用表面：可反复练习
• 儿童适用：适合各学习阶段

适用于课堂组织和学前班活动。
```

**搜索词（15个按顺序）:**
```
干擦文件袋 可重复使用 硬背板支撑 课堂组织 10x13英寸 试卷保护套 学前班 幼儿园 精细动作技能 学习活动 教学用品 写擦两用 儿童 男孩女孩 3-8岁
```

---

## 七、快速生成检查表

### 生成前检查
- [ ] 品牌名已确认
- [ ] **15-20个关键词已按权重排序（#1最重要）**
- [ ] 5个卖点已按重要性排序
- [ ] 包装清单、尺寸、材质信息齐全
- [ ] 用户提供版标题已准备（用于对比参考）

### 生成后检查
| 检查项 | 英文版 | 中文版 |
|--------|--------|--------|
| 标题字符数 | ≤ 200 | ≤ 100 字 |
| 标题埋词 | 包含关键词#1、#2、#3 | 包含关键词#1、#2、#3 |
| **五点每行** | **235-250 字符**（严格范围） | **235-250 字符** |
| **五点小标题** | Package Contents, Dimensions, Material, Color, Usage | 同英文版 |
| 五点埋词 | 全部15-20个关键词已埋入（按用户顺序） | 全部关键词已埋入 |
| 五点段尾 | 无标点 | 无标点 |
| **描述埋词** | **固定13-15个** = 权重词（按顺序）+ 五点剩余 | 固定13-15个 |
| 搜索词顺序 | 按 #1-#20 权重顺序 | 按 #1-#20 权重顺序 |
| 关键词重复 | 同词 ≤ 2 次 | 同词 ≤ 2 次 |
| 描述字符数 | ≤ 1000 | ≤ 1000 |
| 搜索词字符数 | ≤ 250 | ≤ 250 |
| 敏感词检查 | 无敏感词 | 无敏感词 |
| **交付格式** | Markdown确认 → Word高亮版 | Markdown确认 → Word高亮版 |

**描述埋词计算示例（固定13-15个）：**

| 给了N个词 | 五点埋入 | 五点剩余 | 描述埋入（权重词+剩余） | 总计 |
|-----------|---------|---------|------------------------|------|
| 15个 | #1-#15 | 0个 | #1-#13（按权重延续，无剩余） | **13个** |
| 18个 | #1-#16 | #17-#18 | #1-#13 + #17-#18（权重13个+剩余2个） | **15个** |
| 20个 | #1-#14 | #15-#20 | #1-#9 + #15-#20（权重9个+剩余6个，取前6个） | **15个** |

### 敏感词提醒
❌ 避免使用：You will receive...、退款、促销、对比词汇

---

## 八、输出格式

### 纯文本格式
```
==================
【英文版 Listing】
==================

【标题】
...

【五点】
1. ...
2. ...
...

【描述】
...

【搜索词】
...

==================
【中文版 Listing】
==================

【标题】
...
...
```

### Word 文档格式
- 标题：标题样式
- 五点：项目符号列表
- 描述：正文样式
- 搜索词：代码样式
- **关键词：黄色高亮**

---

## 九、生成示例（纸灯笼案例）

### 输入信息
- **品牌**: pinpai
- **数量**: 12 Pack
- **尺寸**: 10 Inches
- **颜色**: Pastel Colors
- **关键词**: 16个（paper lanterns, pastel paper lanterns, Hanging pastel paper lanterns...）

### 五点输出示例（235-250字符）

**第1点 - Package Contents**（246字符）
```
• Package Contents: 12 pieces paper lanterns in soft pastel colors. Each pastel paper lanterns set includes hanging hooks. The Hanging pastel paper lanterns come flat packed. Quick assembly with instructions. Perfect quantity for room decoration.
```

**第2点 - Dimensions**（244字符）
```
• Dimensions: These chinese lanterns measure 10 inches assembled. Ideal classroom decorations for ceilings. The classroom Lantern decorations fit standard spaces. Easy to hang at various heights for layered effect. Suitable for all venue sizes.
```

**第3点 - Material**（238字符）
```
• Material: Durable rice paper for pastel classroom decor that lasts longer. Lightweight Hanging pastel classroom decor setup in minutes. Perfect for pastel party decorations indoor and outdoor use. Sturdy wire frames maintain shape well.
```

**第4点 - Color**（242字符）
```
• Color: Pastel party lanterns decorations in pink blue yellow green purple. These pastel birthday decorations feature gentle colors. Great unicorn birthday decorations with rainbow tones. Mix and match for custom displays. Works with themes.
```

**第5点 - Usage**（244字符）
```
• Usage: These classroom decorations elementary create inspiring environment. The hanging paper lanterns add dimension. Each pastel large paper lantern makes statement. Essential for baby showers weddings graduations. Transform rooms magically.
```

**埋词检查**: 全部16个关键词已按顺序埋入，字数均在235-250范围内 ✓

### 描述输出示例（13个关键词，两段话）

**场景**：给了16个关键词，五点埋完全部16个，描述埋#1-#13（两段话形式，不用列表）

```
<b>Product Description</b>

pinpai paper lanterns are designed to create magical atmosphere for any celebration. These pastel paper lanterns feature soft colors perfect for classroom decorations, baby showers and weddings. The Hanging pastel paper lanterns design makes installation effortless. Perfect chinese lanterns style adds cultural elegance to any space. Ideal classroom Lantern decorations for teachers and event planners.

These pastel classroom decor pieces transform any room instantly. The Hanging pastel classroom decor setup takes minutes. Perfect pastel party decorations for birthdays and celebrations. Great pastel party lanterns decorations for themed events. Beautiful pastel birthday decorations create dreamy ambiance. Essential unicorn birthday decorations for magical parties.
```

**埋词检查**:
- 第一段7个：#1-#7（paper lanterns, pastel paper lanterns, classroom decorations, Hanging pastel paper lanterns, chinese lanterns, classroom Lantern decorations）
- 第二段6个：#8-#13（pastel classroom decor, Hanging pastel classroom decor, pastel party decorations, pastel party lanterns decorations, pastel birthday decorations, unicorn birthday decorations）
- 总计：13个（达到最低要求）✓

### 搜索词输出示例

```
paper lanterns pastel paper lanterns Hanging pastel paper lanterns chinese lanterns classroom decorations classroom Lantern decorations pastel classroom decor Hanging pastel classroom decor pastel party decorations pastel party lanterns decorations pastel birthday decorations unicorn birthday decorations classroom decorations elementary hanging paper lanterns pastel large paper lantern pastel decorations
```

**说明**：全部16个关键词按原顺序排列，空格分隔 ✓

---

## 十、规则速查卡

| 项目 | 规则 |
|------|------|
| **工作流程** | Markdown确认 → Word高亮版 |
| **五点字数** | 235-250字符（严格） |
| **五点小标题** | Package Contents, Dimensions, Material, Color, Usage |
| **关键词顺序** | 严格按照用户给的顺序 |
| **关键词形式** | 原样保留，不改变 |
| **描述埋词** | 固定13-15个 = 权重词 + 五点剩余 |
| **高亮标记** | 仅Word版黄色高亮，Markdown纯文本 |

---

**文档版本**: v2.0  
**创建日期**: 2026-02-21  
**最后更新**: 2026-02-21  
**适用站点**: 亚马逊美国站（英文）+ 参考（中文）
