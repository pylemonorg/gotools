package obsutil

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pylemonorg/gotools/logger"

	obs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
)

// OBS 相关的哨兵错误。
var (
	ErrObsNilConfig        = errors.New("obsutil: 配置不能为 nil")
	ErrObjectAlreadyExists = errors.New("obsutil: 对象已存在")
)

// ObsClient 封装了华为云 OBS 客户端，提供便捷的对象存储操作。
type ObsClient struct {
	client   *obs.ObsClient
	bucket   string
	endpoint string
}

// ObsConfig 定义 OBS 连接所需的参数。
type ObsConfig struct {
	AccessKeyID     string // AK
	SecretAccessKey string // SK
	Endpoint        string // 端点，如 https://obs.cn-north-4.myhuaweicloud.com
	Bucket          string // 存储桶名称
}

// Validate 校验 OBS 配置参数的必填项。
func (c *ObsConfig) Validate() error {
	var missing []string
	if strings.TrimSpace(c.AccessKeyID) == "" {
		missing = append(missing, "AccessKeyID")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		missing = append(missing, "SecretAccessKey")
	}
	if strings.TrimSpace(c.Endpoint) == "" {
		missing = append(missing, "Endpoint")
	}
	if strings.TrimSpace(c.Bucket) == "" {
		missing = append(missing, "Bucket")
	}
	if len(missing) > 0 {
		return fmt.Errorf("obsutil: 缺少必要连接参数: %s", strings.Join(missing, ", "))
	}
	return nil
}

// NewObsClient 根据给定配置创建 ObsClient 实例。
func NewObsClient(cfg *ObsConfig) (*ObsClient, error) {
	if cfg == nil {
		return nil, ErrObsNilConfig
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client, err := obs.New(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 创建客户端失败: %w", err)
	}

	logger.Infof("obsutil: 连接成功 bucket=%s endpoint=%s", cfg.Bucket, cfg.Endpoint)
	return &ObsClient{
		client:   client,
		bucket:   cfg.Bucket,
		endpoint: cfg.Endpoint,
	}, nil
}

// NewObsClientFromEnv 从环境变量创建 ObsClient 实例。
// 读取的环境变量：OBS_AK / AccessKeyID、OBS_SK / SecretAccessKey、OBS_ENDPOINT、OBS_BUCKET。
func NewObsClientFromEnv() (*ObsClient, error) {
	ak := os.Getenv("OBS_AK")
	if ak == "" {
		ak = os.Getenv("AccessKeyID")
	}
	sk := os.Getenv("OBS_SK")
	if sk == "" {
		sk = os.Getenv("SecretAccessKey")
	}

	return NewObsClient(&ObsConfig{
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		Endpoint:        os.Getenv("OBS_ENDPOINT"),
		Bucket:          os.Getenv("OBS_BUCKET"),
	})
}

// Close 关闭 OBS 客户端连接。
func (oc *ObsClient) Close() {
	if oc.client != nil {
		oc.client.Close()
		logger.Infof("obsutil: 客户端连接已关闭")
	}
}

// GetBucket 返回存储桶名称。
func (oc *ObsClient) GetBucket() string { return oc.bucket }

// GetEndpoint 返回端点地址。
func (oc *ObsClient) GetEndpoint() string { return oc.endpoint }

// GetClient 返回底层 obs.ObsClient，可用于执行未封装的高级操作。
func (oc *ObsClient) GetClient() *obs.ObsClient { return oc.client }

// ---------------------------------------------------------------------------
// 上传操作
// ---------------------------------------------------------------------------

// PutFile 上传本地文件到 OBS。
func (oc *ObsClient) PutFile(key, filePath string) (*obs.PutObjectOutput, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("obsutil: 文件不存在: %s", filePath)
	}

	fd, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 打开文件失败: %w", err)
	}
	defer fd.Close()

	input := &obs.PutObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key
	input.Body = fd

	output, err := oc.client.PutObject(input)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 上传文件失败: %w", err)
	}
	return output, nil
}

// PutObject 上传 io.Reader 数据流到 OBS。
func (oc *ObsClient) PutObject(key string, body io.Reader) (*obs.PutObjectOutput, error) {
	input := &obs.PutObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key
	input.Body = body

	output, err := oc.client.PutObject(input)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 上传对象失败: %w", err)
	}
	return output, nil
}

// PutBytes 上传字节数组到 OBS。
func (oc *ObsClient) PutBytes(key string, data []byte) (*obs.PutObjectOutput, error) {
	return oc.PutObject(key, bytes.NewReader(data))
}

// PutString 上传字符串到 OBS。
func (oc *ObsClient) PutString(key, content string) (*obs.PutObjectOutput, error) {
	return oc.PutBytes(key, []byte(content))
}

// putObjectTimeout 单次 PutObject 超时时间。
const putObjectTimeout = 30 * time.Second

// PutBytesWithRetry 上传字节数组到 OBS，带重试和单次超时（应对 503/限流/无响应）。
// maxRetries <= 0 时默认 3 次，retryDelay <= 0 时默认 1s，之后指数退避。
func (oc *ObsClient) PutBytesWithRetry(key string, data []byte, maxRetries int, retryDelay time.Duration) (*obs.PutObjectOutput, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := retryDelay * time.Duration(1<<uint(attempt-1))
			logger.Warnf("obsutil: PutBytes 重试 (%d/%d) key=%s", attempt, maxRetries, key)
			time.Sleep(delay)
		}

		type putResult struct {
			out *obs.PutObjectOutput
			err error
		}
		ch := make(chan putResult, 1)
		go func() {
			out, err := oc.PutObject(key, bytes.NewReader(data))
			select {
			case ch <- putResult{out, err}:
			default:
			}
		}()

		select {
		case r := <-ch:
			if r.err == nil {
				return r.out, nil
			}
			lastErr = r.err
			if attempt < maxRetries && isRetryable(r.err) {
				continue
			}
			return nil, r.err
		case <-time.After(putObjectTimeout):
			lastErr = fmt.Errorf("obsutil: PutObject 超时(%v)", putObjectTimeout)
			if attempt < maxRetries {
				continue
			}
			return nil, lastErr
		}
	}
	return nil, fmt.Errorf("obsutil: 上传失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// PutStringWithRetry 上传字符串到 OBS，带重试机制。
// maxRetries <= 0 时默认 3 次，retryDelay <= 0 时默认 2s，之后指数退避。
func (oc *ObsClient) PutStringWithRetry(key, content string, maxRetries int, retryDelay time.Duration) (*obs.PutObjectOutput, error) {
	return oc.PutBytesWithRetry(key, []byte(content), maxRetries, retryDelay)
}

// PutBytesMultipart 分段并行上传字节数组（适用于大文件）。
// partSize <= 0 时默认 50MB，concurrency <= 0 时默认 5。
func (oc *ObsClient) PutBytesMultipart(key string, data []byte, partSize int64, concurrency int) error {
	dataLen := int64(len(data))
	if partSize <= 0 {
		partSize = 50 * 1024 * 1024
	}
	if concurrency <= 0 {
		concurrency = 5
	}

	// 小文件直接普通上传
	if dataLen <= partSize {
		_, err := oc.PutBytes(key, data)
		return err
	}

	// 初始化分段上传
	initInput := &obs.InitiateMultipartUploadInput{}
	initInput.Bucket = oc.bucket
	initInput.Key = key

	initOutput, err := oc.client.InitiateMultipartUpload(initInput)
	if err != nil {
		return fmt.Errorf("obsutil: 初始化分段上传失败: %w", err)
	}
	uploadID := initOutput.UploadId
	partCount := int((dataLen + partSize - 1) / partSize)

	// 并发上传分段
	type partResult struct {
		PartNumber int
		ETag       string
		Err        error
	}
	results := make(chan partResult, partCount)
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for i := 0; i < partCount; i++ {
		wg.Add(1)
		go func(partNum int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := int64(partNum) * partSize
			end := start + partSize
			if end > dataLen {
				end = dataLen
			}

			uploadInput := &obs.UploadPartInput{}
			uploadInput.Bucket = oc.bucket
			uploadInput.Key = key
			uploadInput.UploadId = uploadID
			uploadInput.PartNumber = partNum + 1
			uploadInput.Body = bytes.NewReader(data[start:end])

			output, err := oc.client.UploadPart(uploadInput)
			if err != nil {
				results <- partResult{PartNumber: partNum + 1, Err: err}
				return
			}
			results <- partResult{PartNumber: partNum + 1, ETag: output.ETag}
		}(i)
	}
	go func() { wg.Wait(); close(results) }()

	// 收集结果
	parts := make([]obs.Part, 0, partCount)
	var uploadErr error
	for r := range results {
		if r.Err != nil {
			uploadErr = r.Err
			continue
		}
		parts = append(parts, obs.Part{PartNumber: r.PartNumber, ETag: r.ETag})
	}

	// 有失败则取消
	if uploadErr != nil || len(parts) != partCount {
		oc.abortMultipartUpload(key, uploadID)
		if uploadErr != nil {
			return fmt.Errorf("obsutil: 分段上传失败: %w", uploadErr)
		}
		return fmt.Errorf("obsutil: 分段不完整: 期望 %d 个，实际 %d 个", partCount, len(parts))
	}

	// 按分段号排序并完成上传
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

	completeInput := &obs.CompleteMultipartUploadInput{}
	completeInput.Bucket = oc.bucket
	completeInput.Key = key
	completeInput.UploadId = uploadID
	completeInput.Parts = parts

	if _, err = oc.client.CompleteMultipartUpload(completeInput); err != nil {
		return fmt.Errorf("obsutil: 完成分段上传失败: %w", err)
	}
	return nil
}

// abortMultipartUpload 取消分段上传（内部辅助方法）。
func (oc *ObsClient) abortMultipartUpload(key, uploadID string) {
	abortInput := &obs.AbortMultipartUploadInput{}
	abortInput.Bucket = oc.bucket
	abortInput.Key = key
	abortInput.UploadId = uploadID
	oc.client.AbortMultipartUpload(abortInput)
}

// ---------------------------------------------------------------------------
// 下载 / 查询操作
// ---------------------------------------------------------------------------

// GetObject 下载对象内容到内存。
func (oc *ObsClient) GetObject(key string) ([]byte, error) {
	input := &obs.GetObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key

	output, err := oc.client.GetObject(input)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 下载对象失败: %w", err)
	}
	defer output.Body.Close()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 读取对象内容失败: %w", err)
	}
	return data, nil
}

// DownloadObject 下载对象到本地文件。
func (oc *ObsClient) DownloadObject(key, filePath string) error {
	input := &obs.GetObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key

	output, err := oc.client.GetObject(input)
	if err != nil {
		return fmt.Errorf("obsutil: 下载对象失败: %w", err)
	}
	defer output.Body.Close()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("obsutil: 创建本地文件失败: %w", err)
	}
	defer file.Close()

	if _, err = io.Copy(file, output.Body); err != nil {
		return fmt.Errorf("obsutil: 写入本地文件失败: %w", err)
	}
	return nil
}

// ObjectExists 检查对象是否存在。404 返回 false,nil；其他错误返回 false,err。
func (oc *ObsClient) ObjectExists(key string) (bool, error) {
	input := &obs.HeadObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key

	if _, err := oc.client.HeadObject(input); err != nil {
		if obsErr, ok := err.(obs.ObsError); ok && obsErr.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("obsutil: 检查对象是否存在失败: %w", err)
	}
	return true, nil
}

// ObjectExistsWithRetry 检查对象是否存在，带重试（应对限流/网络抖动）。
// 404 不重试，直接返回 false,nil。
func (oc *ObsClient) ObjectExistsWithRetry(key string, maxRetries int, retryDelay time.Duration) (bool, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	input := &obs.HeadObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay * time.Duration(1<<uint(attempt-1)))
		}

		if _, err := oc.client.HeadObject(input); err == nil {
			return true, nil
		} else if obsErr, ok := err.(obs.ObsError); ok && obsErr.StatusCode == 404 {
			return false, nil
		} else {
			lastErr = err
			if attempt < maxRetries && isRetryable(err) {
				continue
			}
			return false, fmt.Errorf("obsutil: 检查对象是否存在失败: %w", lastErr)
		}
	}
	return false, fmt.Errorf("obsutil: 检查对象是否存在失败: %w", lastErr)
}

// ---------------------------------------------------------------------------
// 删除 / 复制操作
// ---------------------------------------------------------------------------

// DeleteObject 删除单个对象。
func (oc *ObsClient) DeleteObject(key string) (*obs.DeleteObjectOutput, error) {
	input := &obs.DeleteObjectInput{}
	input.Bucket = oc.bucket
	input.Key = key

	output, err := oc.client.DeleteObject(input)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 删除对象失败: %w", err)
	}
	return output, nil
}

// DeleteObjects 批量删除对象（自动分批，每批最多 1000 个）。
// 返回成功删除的数量、失败的 key 列表。
func (oc *ObsClient) DeleteObjects(keys []string) (int, []string, error) {
	if len(keys) == 0 {
		return 0, nil, nil
	}

	const batchSize = 1000
	var totalSuccess int
	var allFailed []string

	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[i:end]

		success, failed, err := oc.deleteObjectsBatch(batch)
		if err != nil {
			allFailed = append(allFailed, batch...)
			continue
		}
		totalSuccess += success
		allFailed = append(allFailed, failed...)
	}
	return totalSuccess, allFailed, nil
}

// deleteObjectsBatch 删除单批对象（内部方法）。
func (oc *ObsClient) deleteObjectsBatch(keys []string) (int, []string, error) {
	objects := make([]obs.ObjectToDelete, len(keys))
	for i, key := range keys {
		objects[i] = obs.ObjectToDelete{Key: key}
	}

	input := &obs.DeleteObjectsInput{}
	input.Bucket = oc.bucket
	input.Objects = objects
	input.Quiet = false

	output, err := oc.client.DeleteObjects(input)
	if err != nil {
		return 0, keys, fmt.Errorf("obsutil: 批量删除失败: %w", err)
	}

	var failed []string
	for _, e := range output.Errors {
		failed = append(failed, e.Key)
	}
	return len(output.Deleteds), failed, nil
}

// CopyObject 在同一存储桶内复制对象。
func (oc *ObsClient) CopyObject(srcKey, destKey string) error {
	input := &obs.CopyObjectInput{}
	input.Bucket = oc.bucket
	input.Key = destKey
	input.CopySourceBucket = oc.bucket
	input.CopySourceKey = srcKey

	if _, err := oc.client.CopyObject(input); err != nil {
		return fmt.Errorf("obsutil: 复制对象失败: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 列表操作
// ---------------------------------------------------------------------------

// ListObjects 列出指定前缀的对象（单页）。maxKeys <= 0 时默认 1000。
func (oc *ObsClient) ListObjects(prefix string, maxKeys int) ([]obs.Content, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	input := &obs.ListObjectsInput{}
	input.Bucket = oc.bucket
	input.Prefix = prefix
	input.MaxKeys = maxKeys

	output, err := oc.client.ListObjects(input)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 列出对象失败: %w", err)
	}
	return output.Contents, nil
}

// ListObjectsWithMarker 带分页标记列出对象。
// 返回对象列表和下一页 marker（空串表示无更多数据）。
func (oc *ObsClient) ListObjectsWithMarker(prefix string, maxKeys int, marker string) ([]obs.Content, string, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}

	input := &obs.ListObjectsInput{}
	input.Bucket = oc.bucket
	input.Prefix = prefix
	input.MaxKeys = maxKeys
	input.Marker = marker

	output, err := oc.client.ListObjects(input)
	if err != nil {
		return nil, "", fmt.Errorf("obsutil: 列出对象失败: %w", err)
	}

	nextMarker := ""
	if output.IsTruncated && len(output.Contents) > 0 {
		nextMarker = output.Contents[len(output.Contents)-1].Key
	}
	return output.Contents, nextMarker, nil
}

// ListAllObjects 自动分页列出所有对象。
func (oc *ObsClient) ListAllObjects(prefix string, maxKeysPerPage int) ([]obs.Content, error) {
	return oc.ListAllObjectsWithProgress(prefix, maxKeysPerPage, nil, 0)
}

// ListAllObjectsWithProgress 自动分页列出对象，支持进度回调和数量限制。
// progressCallback 在每获取一页后回调，参数为当前累计数量。maxCount 为 0 表示不限制。
func (oc *ObsClient) ListAllObjectsWithProgress(prefix string, maxKeysPerPage int, progressCallback func(int), maxCount int) ([]obs.Content, error) {
	if maxKeysPerPage <= 0 {
		maxKeysPerPage = 1000
	}
	if maxKeysPerPage > 10000 {
		maxKeysPerPage = 10000
	}

	var allObjects []obs.Content
	var marker string

	for {
		pageSize := maxKeysPerPage
		if maxCount > 0 {
			if remaining := maxCount - len(allObjects); remaining < pageSize {
				pageSize = remaining
			}
		}

		input := &obs.ListObjectsInput{}
		input.Bucket = oc.bucket
		input.Prefix = prefix
		input.MaxKeys = pageSize
		input.Marker = marker

		output, err := oc.client.ListObjects(input)
		if err != nil {
			return nil, fmt.Errorf("obsutil: 列出对象失败: %w", err)
		}

		if maxCount > 0 {
			remaining := maxCount - len(allObjects)
			if len(output.Contents) > remaining {
				allObjects = append(allObjects, output.Contents[:remaining]...)
			} else {
				allObjects = append(allObjects, output.Contents...)
			}
		} else {
			allObjects = append(allObjects, output.Contents...)
		}

		if progressCallback != nil {
			progressCallback(len(allObjects))
		}

		if maxCount > 0 && len(allObjects) >= maxCount {
			break
		}
		if !output.IsTruncated || len(output.Contents) == 0 {
			break
		}

		marker = output.Contents[len(output.Contents)-1].Key
	}
	return allObjects, nil
}

// ---------------------------------------------------------------------------
// 分布式锁
// ---------------------------------------------------------------------------

// TryCreateLock 尝试创建 OBS 锁文件（简易分布式锁）。
// 先检查是否存在 → 创建锁 → 验证锁属于自己。
// 成功返回 true,nil；锁被其他实例持有返回 false,nil。
func (oc *ObsClient) TryCreateLock(key string, lockContent []byte, instanceID string) (bool, error) {
	exists, err := oc.ObjectExists(key)
	if err != nil {
		return false, fmt.Errorf("obsutil: 检查锁文件失败: %w", err)
	}
	if exists {
		return false, nil
	}

	if _, err = oc.PutObject(key, bytes.NewReader(lockContent)); err != nil {
		return false, fmt.Errorf("obsutil: 创建锁文件失败: %w", err)
	}

	// 等待 OBS 最终一致性生效
	time.Sleep(50 * time.Millisecond)

	data, err := oc.GetObject(key)
	if err != nil {
		return false, fmt.Errorf("obsutil: 读取锁文件失败: %w", err)
	}

	if !bytes.Contains(data, []byte(instanceID)) {
		return false, nil
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// 流式分段上传
// ---------------------------------------------------------------------------

// StreamingUploader 流式分段上传器，边写边上传，减少内存占用。
//
// 用法：
//
//	uploader, _ := obsClient.NewStreamingUploader("path/to/file")
//	uploader.WritePart(chunk1)
//	uploader.WritePart(chunk2)
//	uploader.Complete()  // 或失败时 uploader.Abort()
type StreamingUploader struct {
	obsClient  *ObsClient
	key        string
	uploadID   string
	parts      []obs.Part
	partNumber int
	mu         sync.Mutex
	aborted    bool
	completed  bool
}

// NewStreamingUploader 创建流式上传器。
func (oc *ObsClient) NewStreamingUploader(key string) (*StreamingUploader, error) {
	initInput := &obs.InitiateMultipartUploadInput{}
	initInput.Bucket = oc.bucket
	initInput.Key = key

	initOutput, err := oc.client.InitiateMultipartUpload(initInput)
	if err != nil {
		return nil, fmt.Errorf("obsutil: 初始化分段上传失败: %w", err)
	}

	return &StreamingUploader{
		obsClient: oc,
		key:       key,
		uploadID:  initOutput.UploadId,
		parts:     make([]obs.Part, 0),
	}, nil
}

// WritePart 上传一个分段（建议 10MB-100MB）。线程安全。
func (su *StreamingUploader) WritePart(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	su.mu.Lock()
	if su.aborted {
		su.mu.Unlock()
		return errors.New("obsutil: 上传已取消")
	}
	if su.completed {
		su.mu.Unlock()
		return errors.New("obsutil: 上传已完成")
	}
	su.partNumber++
	partNum := su.partNumber
	su.mu.Unlock()

	// 带重试上传
	var lastErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Second * time.Duration(retry))
		}

		uploadInput := &obs.UploadPartInput{}
		uploadInput.Bucket = su.obsClient.bucket
		uploadInput.Key = su.key
		uploadInput.UploadId = su.uploadID
		uploadInput.PartNumber = partNum
		uploadInput.Body = bytes.NewReader(data)

		output, err := su.obsClient.client.UploadPart(uploadInput)
		if err != nil {
			lastErr = err
			continue
		}

		su.mu.Lock()
		su.parts = append(su.parts, obs.Part{PartNumber: partNum, ETag: output.ETag})
		su.mu.Unlock()
		return nil
	}
	return fmt.Errorf("obsutil: 分段 %d 上传失败（重试3次）: %w", partNum, lastErr)
}

// Complete 完成分段上传，合并所有分段。
func (su *StreamingUploader) Complete() error {
	su.mu.Lock()
	defer su.mu.Unlock()

	if su.aborted {
		return errors.New("obsutil: 上传已取消")
	}
	if su.completed {
		return nil
	}
	if len(su.parts) == 0 {
		return errors.New("obsutil: 没有上传任何分段")
	}
	if len(su.parts) != su.partNumber {
		return fmt.Errorf("obsutil: 分段不完整: 期望 %d 个，实际 %d 个", su.partNumber, len(su.parts))
	}

	sort.Slice(su.parts, func(i, j int) bool { return su.parts[i].PartNumber < su.parts[j].PartNumber })

	completeInput := &obs.CompleteMultipartUploadInput{}
	completeInput.Bucket = su.obsClient.bucket
	completeInput.Key = su.key
	completeInput.UploadId = su.uploadID
	completeInput.Parts = su.parts

	if _, err := su.obsClient.client.CompleteMultipartUpload(completeInput); err != nil {
		return fmt.Errorf("obsutil: 完成分段上传失败: %w", err)
	}
	su.completed = true
	return nil
}

// Abort 取消分段上传，清理已上传的临时分段。
func (su *StreamingUploader) Abort() error {
	su.mu.Lock()
	defer su.mu.Unlock()

	if su.completed || su.aborted {
		return nil
	}

	abortInput := &obs.AbortMultipartUploadInput{}
	abortInput.Bucket = su.obsClient.bucket
	abortInput.Key = su.key
	abortInput.UploadId = su.uploadID

	if _, err := su.obsClient.client.AbortMultipartUpload(abortInput); err != nil {
		logger.Warnf("obsutil: 取消分段上传失败（OBS 会自动清理）: %v", err)
	}
	su.aborted = true
	return nil
}

// PartsCount 返回已成功上传的分段数。
func (su *StreamingUploader) PartsCount() int {
	su.mu.Lock()
	defer su.mu.Unlock()
	return len(su.parts)
}

// TotalPartNumber 返回总分段数（含失败的）。
func (su *StreamingUploader) TotalPartNumber() int {
	su.mu.Lock()
	defer su.mu.Unlock()
	return su.partNumber
}

// ---------------------------------------------------------------------------
// 内部辅助函数
// ---------------------------------------------------------------------------

// retryableKeywords 可重试的错误关键词。
var retryableKeywords = []string{
	"503", "Service Unavailable",
	"GetQosTokenException", "connection token",
	"429", "Too Many Requests", "Indicator=601",
	"timeout", "connection", "wsarecv",
	"i/o timeout", "connection reset",
}

// isRetryable 判断错误是否可重试（限流/临时不可用/网络问题）。
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, kw := range retryableKeywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
