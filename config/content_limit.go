package config

const PostContentMaxRunes = 10000

// CommentContentMaxRunes 集市帖子评论/楼中楼正文字符上限（与前端评论输入框一致）。
// DB 列 pcomments.pctext / ccomments.cctext 为 varchar(4000)，见 model 与迁移文档。
const CommentContentMaxRunes = 4000
