package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"github.com/tencentyun/cos-go-sdk-v5"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type getFile struct {
	Key        string `json:"key"`
	Name       string `json:"Name"`
	EditTime   string `json:"EditTime"`
	Type       int    `json:"Type"`
	Size       int64  `json:"Size"`
	SuffixName string `json:"SuffixName"`
}

func getClient() (*cos.Client, string) {
	// 存储桶名称，由 bucketname-appid 组成，appid 必须填入，可以在 COS 控制台查看存储桶名称。 https://console.cloud.tencent.com/cos5/bucket
	// 替换为用户的 region，存储桶 region 可以在 COS 控制台“存储桶概览”查看 https://console.cloud.tencent.com/ ，关于地域的详情见 https://cloud.tencent.com/document/product/436/6224 。
	bucketName := viper.GetString("cos.bucketName")
	appid := viper.GetString("cos.appid")
	args := fmt.Sprintf("https://%s-%s.cos.ap-guangzhou.myqcloud.com", bucketName, appid)
	u, _ := url.Parse(args)
	b := &cos.BaseURL{BucketURL: u}
	return cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			// 通过环境变量获取密钥
			// 环境变量 SECRETID 表示用户的 SecretId，登录访问管理控制台查看密钥，https://console.cloud.tencent.com/cam/capi
			SecretID: viper.GetString("cos.secretId"), // 用户的 SecretId，建议使用子账号密钥，授权遵循最小权限指引，降低使用风险。子账号密钥获取可参见 https://cloud.tencent.com/document/product/598/37140
			// 环境变量 SECRETKEY 表示用户的 SecretKey，登录访问管理控制台查看密钥，https://console.cloud.tencent.com/cam/capi
			SecretKey: viper.GetString("cos.secretKey"), // 用户的 SecretKey，建议使用子账号密钥，授权遵循最小权限指引，降低使用风险。子账号密钥获取可参见 https://cloud.tencent.com/document/product/598/37140
		},
	}), args
}

func UploadImage(_ string, localpath string) (string, error) {
	uploadURL := viper.GetString("imagebed.upload_url")
	if uploadURL == "" {
		uploadURL = "https://image.ssemarket.cn/api/upload"
	}
	token := viper.GetString("imagebed.token")

	file, err := os.Open(localpath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(localpath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("imagebed upload failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("imagebed upload response parse failed: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("imagebed upload returned code=%d message=%s", result.Code, result.Message)
	}
	if len(result.Data) == 0 || result.Data[0].URL == "" {
		return "", fmt.Errorf("imagebed upload response missing url")
	}

	newURL := result.Data[0].URL
	if strings.HasPrefix(newURL, "/") {
		base := viper.GetString("imagebed.public_base")
		if base == "" {
			base = "https://image.ssemarket.cn"
		}
		newURL = strings.TrimRight(base, "/") + newURL
	}
	return newURL, nil
}
func AddFolder(key string) error {
	client, _ := getClient()
	_, err := client.Object.Put(context.Background(), key, strings.NewReader(""), nil)
	return err
}
func UploadFile(key string, file io.Reader) error {
	client, _ := getClient()
	_, err := client.Object.Put(context.Background(), key, file, nil)
	return err
}
func ListObject(path string) (any, error) {
	client, _ := getClient()
	var marker string
	opt := &cos.BucketGetOptions{
		Prefix:    path, // prefix 表示要查询的文件夹
		Delimiter: "/",  // deliter 表示分隔符, 设置为/表示列出当前目录下的 object, 设置为空表示列出所有的 object
		MaxKeys:   1000, // 设置最大遍历出多少个对象, 一次 listobject 最大支持1000
	}
	isTruncated := true
	var files []getFile
	for isTruncated {
		opt.Marker = marker
		v, _, err := client.Bucket.Get(context.Background(), opt)
		if err != nil {
			fmt.Println(err)
			return nil, err
			break
		}
		log.Println("prefix:" + v.Prefix)
		log.Println("parent" + getParentPath(v.Prefix))
		if strings.Count(v.Prefix, "/") >= 3 {
			files = append(files, getFile{
				Key:        getParentPath(v.Prefix),
				Name:       "上一级目录",
				EditTime:   "",
				Type:       1,
				Size:       0,
				SuffixName: "",
			})
		}
		// common prefix 表示表示被 delimiter 截断的路径, 如 delimter 设置为/, common prefix 则表示所有子目录的路径
		for _, commonPrefix := range v.CommonPrefixes {
			fmt.Printf("CommonPrefixes: %v\n", commonPrefix)
			//添加文件夹
			files = append(files, getFile{
				Key:        commonPrefix,
				Name:       filepath.Base(commonPrefix),
				EditTime:   "",
				Type:       1,
				Size:       0,
				SuffixName: "",
			})
		}
		for _, content := range v.Contents {
			log.Println("key:" + content.Key)
			if content.Key == v.Prefix {
				// 创建一个新的 getFile 结构
				//newFile := getFile{
				//	Key:        getParentPath(content.Key),
				//	Name:       "上一级目录",
				//	EditTime:   "",
				//	Type:       1,
				//	Size:       content.Size,
				//	SuffixName: "",
				//}
				// 使用切片切割操作将新文件插入到切片开头
				//files = append([]getFile{newFile}, files...)
			} else {
				ext := filepath.Ext(content.Key)
				cleanedExt := strings.TrimPrefix(ext, ".")
				// cleanedExt 现在包含不带点号的文件扩展名
				files = append(files, getFile{
					Key:      content.Key,
					Name:     filepath.Base(content.Key),
					EditTime: content.LastModified,
					Type:     0,
					// 由于前端最小单位是KB，所以为了显示的适应，这里修改一下
					Size:       content.Size / 1024,
					SuffixName: cleanedExt,
				})
			}
		}

		isTruncated = v.IsTruncated // 是否还有数据
		marker = v.NextMarker       // 设置下次请求的起始 key
	}
	return files, nil
}
func getParentPath(path string) string {
	// 使用filepath包的Dir函数获取路径的上一级目录
	parentDir := filepath.Dir(path)
	parentDir = filepath.Dir(parentDir)
	if parentDir == "." {
		parentDir = ""
	} else {
		// 在window上会出现\，为了防止在不同平台的差异，采用ToSlash全换为“/”
		parentDir = filepath.ToSlash(parentDir) + "/"
	}
	log.Printf("path:" + path + " dir:" + parentDir)
	// 如果路径以斜杠结尾，需要再次获取上一级目录
	//if parentDir != "." && parentDir[len(parentDir)-1] == '/' {
	//	parentDir = filepath.Dir(parentDir)
	//}

	return parentDir
}
func FileDelete(filename string) error {
	client, _ := getClient()
	_, err := client.Object.Delete(context.Background(), filename)
	return err
}
func FolderDelete(dir string) error {
	client, _ := getClient()
	var marker string
	opt := &cos.BucketGetOptions{
		Prefix:  dir,
		MaxKeys: 1000,
	}
	isTruncated := true
	for isTruncated {
		opt.Marker = marker
		v, _, err := client.Bucket.Get(context.Background(), opt)
		if err != nil {
			// Error
			return err
		}
		for _, content := range v.Contents {
			_, err = client.Object.Delete(context.Background(), content.Key)
			if err != nil {
				// Error
				return err
			}
		}
		isTruncated = v.IsTruncated
		marker = v.NextMarker
	}
	return nil
}
func GetUrl(filename string) string {
	client, _ := getClient()
	oUrl := client.Object.GetObjectURL(filename)
	return oUrl.String()
}

// GetPresignedURL 生成带签名的临时访问链接；inline 为 true 时便于浏览器内嵌预览 PDF。
// GetObject 从 COS 读取对象流，用于服务端代理预览。
func GetObject(key string) (io.ReadCloser, string, error) {
	client, _ := getClient()
	resp, err := client.Object.Get(context.Background(), key, nil)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return resp.Body, contentType, nil
}

func attachmentContentDisposition(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	fallback := "resume" + ext
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, fallback, url.PathEscape(filename))
}

func GetPresignedURL(key string, expire time.Duration, inline bool, attachmentFilename string) (string, error) {
	client, _ := getClient()
	secretID := viper.GetString("cos.secretId")
	secretKey := viper.GetString("cos.secretKey")
	opt := &cos.PresignedURLOptions{}
	query := make(url.Values)
	if inline {
		query.Set("response-content-disposition", "inline")
	} else if attachmentFilename != "" {
		query.Set("response-content-disposition", attachmentContentDisposition(attachmentFilename))
	}
	if len(query) > 0 {
		opt.Query = &query
	}
	presigned, err := client.Object.GetPresignedURL(
		context.Background(),
		http.MethodGet,
		key,
		secretID,
		secretKey,
		expire,
		opt,
	)
	if err != nil {
		return "", err
	}
	return presigned.String(), nil
}
