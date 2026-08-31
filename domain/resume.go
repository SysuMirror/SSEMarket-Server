package domain

import "time"

// Resume 简历实体。
type Resume struct {
	ID          int
	UserID      int
	Title       string
	Description string
	FileName    string
	CosKey      string
	FileSize    int64
	MimeType    string
	DownloadNum int
	ViewNum     int
	IsAnonymous bool
	IsDelete    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ResumeFilter 简历列表筛选。
type ResumeFilter struct {
	UserID     int
	SearchInfo string
	SearchSort string
	Limit      int
	Offset     int
}

// ResumeCreateInput 创建简历记录入参（文件已上传至 COS）。
type ResumeCreateInput struct {
	UserID      int
	Title       string
	Description string
	FileName    string
	CosKey      string
	FileSize    int64
	MimeType    string
	IsAnonymous bool
}

// ResumeUpdateInput 更新简历元数据。
type ResumeUpdateInput struct {
	ResumeID    int
	UserID      int
	Title       string
	Description string
}
