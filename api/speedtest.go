package handler

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()

	fmt.Println("🚀 开始 Cloudflare SpeedTest...")

	// 使用你 bin/ 目录下的 cfst 二进制
	cmd := exec.CommandContext(ctx, "./bin/cfst",
		"-httping",        // 推荐使用 httping 模式
		"-tl", "300",      // 延迟阈值（毫秒）
		"-sl", "8",        // 速度阈值（MB/s）
		"-dn", "8",        // 测速节点数量（建议6-10，太大容易超时）
		"-o", "result.csv",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("执行错误: %v\nOutput: %s\n", err, output)
		http.Error(w, "测速执行失败", 500)
		return
	}

	// 读取结果文件
	data, err := os.ReadFile("result.csv")
	if err != nil {
		http.Error(w, "读取 result.csv 失败", 500)
		return
	}

	// 上传到 R2
	if err := uploadToR2(data); err != nil {
		fmt.Printf("R2 上传失败: %v\n", err)
		http.Error(w, "上传 R2 失败", 500)
		return
	}

	fmt.Fprintln(w, "✅ 测速完成，已成功上传到 Cloudflare R2")
	fmt.Fprint(w, string(output))
}

func uploadToR2(data []byte) error {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("auto"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			os.Getenv("R2_ACCESS_KEY_ID"),
			os.Getenv("R2_SECRET_ACCESS_KEY"), "",
		)),
	)
	if err != nil {
		return err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", os.Getenv("R2_ACCOUNT_ID")))
	})

	filename := fmt.Sprintf("speedtest/%s.csv", time.Now().Format("20060102_150405"))

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(os.Getenv("R2_BUCKET_NAME")),
		Key:         aws.String(filename),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("text/csv"),
	})

	return err
}
