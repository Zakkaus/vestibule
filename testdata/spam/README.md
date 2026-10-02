# 垃圾信息样本

线上收集的真实样本，用作反垃圾规则的测试用例。

**这些样本不进出厂默认规则。** 默认规则要对所有群成立，
而具体的广告词只对特定时期的特定骗局成立。这里保留它们是为了验证
归一化与结构信号能否命中，以及日后改动匹配逻辑时不发生回归。

每份样本记录它包含的规避手法，测试断言的是「归一化之后能否命中」，
不是「是否包含某个词」。

## 已收录的规避手法

| 手法 | 码位 | 断言 |
|---|---|---|
| 零宽空格插词 | U+200B | 归一化后与去除该字符的文本一致 |
| 零宽非连接符插词 | U+200C | 同上 |
| 词连接符插词 | U+2060 | 同上 |
| 间隔号与星号插词 | `·` `*` `-` | 折叠后一致 |
| 全角字符 | 全角 `＠` 等 | NFKC 后一致 |
| emoji 夹杂 | — | 不影响结构信号计数 |
| 多条私有邀请链接 | — | 链接计数达到阈值 |
| 多个用户名提及 | — | 提及计数达到阈值 |

## Structural-signal fixtures

These fixtures are synthetic, not collected messages. Tests supply entity kinds and
URL targets explicitly; the scorer never discovers entities by scanning the text.

| File | Input shape | Expected default score |
|---|---|---|
| `emoji-sequences.txt` | Family, heart, keycaps, England flag, qualified emoji joiners | 0 |
| `private-invites.txt` | One URL and one text-link entity, both private invites; unknown join time | 8 |
| `mentions.txt` | Two mention entities and one text-mention entity | 3 |
| `combined-signals.txt` | Two hidden characters, two private invites, two mentions; joined one hour ago | 18 |

Default weights are 2 per hidden character, 3 per private invite, 1 per mention,
1 per link and 4 for a known member who joined less than 24 hours ago and posts a link.
At an illustrative threshold of 10, one ordinary link from a long-standing member
scores 1; a new member posting two private invites with one hidden character scores 14.
The pure scorer does not choose a threshold or disposition.
