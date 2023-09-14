package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/viper"
	"github.com/tencentyun/cos-go-sdk-v5"
)

func UploadImage(filepath string, localpath string) (string) {
    // 存储桶名称，由 bucketname-appid 组成，appid 必须填入，可以在 COS 控制台查看存储桶名称。 https://console.cloud.tencent.com/cos5/bucket
    // 替换为用户的 region，存储桶 region 可以在 COS 控制台“存储桶概览”查看 https://console.cloud.tencent.com/ ，关于地域的详情见 https://cloud.tencent.com/document/product/436/6224 。
	bucketName := viper.GetString("cos.bucketName")
	appid := viper.GetString("cos.appid")
	args := fmt.Sprintf("https://%s-%s.cos.ap-guangzhou.myqcloud.com", bucketName, appid)
    u, _ := url.Parse(args)
    b := &cos.BaseURL{BucketURL: u}
    client := cos.NewClient(b, &http.Client{
        Transport: &cos.AuthorizationTransport{
            // 通过环境变量获取密钥
            // 环境变量 SECRETID 表示用户的 SecretId，登录访问管理控制台查看密钥，https://console.cloud.tencent.com/cam/capi
            SecretID: viper.GetString("cos.secretId"),  // 用户的 SecretId，建议使用子账号密钥，授权遵循最小权限指引，降低使用风险。子账号密钥获取可参见 https://cloud.tencent.com/document/product/598/37140
            // 环境变量 SECRETKEY 表示用户的 SecretKey，登录访问管理控制台查看密钥，https://console.cloud.tencent.com/cam/capi
            SecretKey: viper.GetString("cos.secretKey"),  // 用户的 SecretKey，建议使用子账号密钥，授权遵循最小权限指引，降低使用风险。子账号密钥获取可参见 https://cloud.tencent.com/document/product/598/37140
        },
    })

    _, _, err := client.Object.Upload(
        context.Background(), filepath, localpath, nil,
    )
    if err != nil {
        panic(err)
    }
	return args + filepath
}
