package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// global config
var ExportType string = "md"
var SleepTime time.Duration = 10 * time.Second
var Retry10 int = 10
var Retry2 int = 2
var Retry20 int = 20
var shimo_sid string = ""

var (
	ROOT_URL   string = "https://shimo.im/lizard-api/files"
	LIST_URL   string = "https://shimo.im/lizard-api/files?folder=%s"
	EXPORT_URL string = "https://shimo.im/lizard-api/office-gw/files/export?fileGuid=%s&type=%s"
	QUERY_URL  string = "https://shimo.im/lizard-api/office-gw/files/export/progress?taskId=%s"
)

type FileInfo struct {
	Path   string
	Id     string `json:"guid"`
	Title  string `json:"name"`
	Type   string `json:"type"`
	TaskId string
}

type FileList []FileInfo

type DirInfo struct {
	FileInfo
	Dirs  *DirList
	Files *FileList
}

type DirList []DirInfo

type FileResponse []FileInfo

type ExportResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	TaskId  string `json:"taskId,omitempty"`
}

type TaskResponse struct {
	Status int `json:"status"`
	Code   int `json:"code"`
	Data   struct {
		Progress    int    `json:"progress"`
		DownloadUrl string `json:"downloadUrl"`
		FileSize    int    `json:"fileSize"`
		CostTime    int    `json:"costTime"`
	} `json:"data,omitempty"`
}

func (tree DirInfo) String() string {
	str := fmt.Sprintf("type: %s id: %s title: %s path: %s &dirs: %p &files: %p", tree.Type, tree.Id, tree.Title, tree.Path, tree.Dirs, tree.Files)
	return str
}

func (file FileInfo) String() string {
	str := fmt.Sprintf("type: %s id: %s title: %s path: %s", file.Type, file.Id, file.Title, file.Path)
	return str
}

// 发起http请求, 必须设置cookie和refer
func httpRequest(uri string, retry int) ([]byte, error) {
	defaultstr := []byte("http request error occur")

	// 创建一个http.Client
	client := &http.Client{}
	//fmt.Println(uri)

	// 创建一个http.Request
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return defaultstr, err
	}

	req.Header.Set("referer", "https://shimo.im/desktop")

	// 创建一个Cookie
	cookie := &http.Cookie{
		Name:  "shimo_sid",
		Value: shimo_sid,
	}

	// 将Cookie添加到Request中
	req.AddCookie(cookie)

	// 使用Client发送Request
	resp, err := client.Do(req)
	if nil != err {
		return defaultstr, err
	}

	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		if retry > 0 {
			fmt.Println("429 too many requests, retry: ", retry, "...", uri)
			time.Sleep(SleepTime)
			return httpRequest(uri, retry-1)
		}
	}

	if resp.StatusCode != 200 {
		return defaultstr, errors.New("status error: " + resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)

	if nil != err {
		return defaultstr, err
	}

	return body, nil
}

// 获取文件夹结构信息
func httpGetInfo(path string, id string, d *DirList, f *FileList) {
	uri := ROOT_URL
	if id != "" {
		uri = fmt.Sprintf(LIST_URL, id)
	}

	b, err := httpRequest(uri, Retry2)
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	var result FileResponse

	err = json.Unmarshal(b, &result)
	if err != nil {
		fmt.Println(err)
	}
	//fmt.Println("res1:", string(b))
	//fmt.Println("res2:", result)

	for i := range result {

		switch result[i].Type {
		case "folder":
			*d = append(*d, DirInfo{
				FileInfo: FileInfo{
					Path:  path + "/" + result[i].Title,
					Id:    result[i].Id,
					Title: result[i].Title,
					Type:  result[i].Type,
				},
				Dirs:  nil,
				Files: nil})
		case "newdoc":
			*f = append(*f, FileInfo{
				Path:  path + "/" + result[i].Title, //TODO:过滤特殊字符
				Id:    result[i].Id,
				Title: result[i].Title,
				Type:  result[i].Type,
			})
		default:
			//fmt.Println("need add type: ", result[i].TYPE)
		}
	}
}

// 获取导出文件,默认按照md格式
func httpExport(id string) (tid string) {
	exportType := ExportType
	fmt.Println("[httpExport]: start export:")
	uri := fmt.Sprintf(EXPORT_URL, id, exportType)

	b, err := httpRequest(uri, Retry10)
	var result ExportResponse
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	err = json.Unmarshal(b, &result)
	if err != nil {
		fmt.Println(err)
	}

	if result.TaskId == "" {
		panic(errors.New("TaskId empty"))
	}

	fmt.Printf("[TaskId]: %+v\n", result.TaskId)
	return result.TaskId
}

// 查询导出结果
func httpLinkQuery(tid string) string {
	uri := fmt.Sprintf(QUERY_URL, tid)
	fmt.Println("[httpLinkQuery]: start query progress:")
	b, err := httpRequest(uri, Retry20)
	var result TaskResponse
	if err != nil {
		fmt.Println(err)
		panic(err)
	}

	err = json.Unmarshal(b, &result)
	if err != nil {
		//fmt.Println(err)
	}

	fmt.Printf("[Progress]: %+v\n", result.Data.Progress)
	fmt.Printf("[DownloadUrl]: %+v\n", result.Data.DownloadUrl)

	if result.Status != 0 || result.Data.DownloadUrl == "" {
		fmt.Println("progress not complete, retry ... ", uri)
		time.Sleep(2 * time.Second)
		// 针对结果做循环调用，查询是否完成
		return httpLinkQuery(tid)
	}

	return result.Data.DownloadUrl
}

// 下载文件
func httpDownload(uri string) []byte {
	fmt.Println("[httpDownload]: start download:", uri)
	b, err := httpRequest(uri, Retry2)

	if err != nil {
		fmt.Println(err)
	}

	return b
}

// 获取当前层id对应的文件夹和文档列表
func getDirInfo(path, id string, d *DirList, f *FileList) {
	httpGetInfo(path, id, d, f)
}

// 递归构造文件结构树,基于深度优先
func StructTree(tree *DirInfo) {
	dirs := &DirList{}
	files := &FileList{}

	getDirInfo(tree.Path, tree.Id, dirs, files)

	tree.Files = files
	tree.Dirs = dirs

	if dirs != nil {
		for i := range *dirs {
			//node := (*dirs)[i] 此处不能使用变量，会导致只修改局部变量node的值
			StructTree(&(*dirs)[i])
		}
	}
}

// 遍历文件结构
func TraverseTree(tree *DirInfo) {
	node := *tree
	if node.Files != nil {
		fl := *(node.Files)
		for i := range fl {
			fmt.Println("-------------------")
			fmt.Println("[TraverseTree]: ", fl[i])
			tid := httpExport(fl[i].Id)
			(*(node.Files))[i].TaskId = tid

			DiskDownload(fl[i])
			fmt.Println("-------------------")
		}
	}

	if *(node.Dirs) == nil {
		fmt.Println(node.Id, "dir nil")
		return
	}

	// 深度遍历
	dl := *(node.Dirs)
	for i := range dl {
		TraverseTree(&dl[i])
	}
}

// title重复时,累计添加(1)
func duplicateTitle(path string) string {
	_, err := os.Stat(path)
	if err == nil {
		path = path + "(1)"
		path = duplicateTitle(path)
	} else if os.IsNotExist(err) {

	} else {
		panic(errors.New("duplicateTitle error "))
	}
	return path
}

// 将f下载到磁盘
func DiskDownload(f FileInfo) {
	dl := httpLinkQuery(f.TaskId)
	b := httpDownload(dl)

	dir := filepath.Dir(f.Path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}

	path := duplicateTitle(f.Path)

	file, _ := os.Create(path + "." + ExportType)
	defer file.Close()

	file.Write(b)

}

// 测试下载f.Id="Wr3DpD6VojTG953J"
func test_download() {
	tid := httpExport("Wr3DpD6VojTG953J")
	dl := httpLinkQuery(tid)
	fmt.Println(dl)
	b := httpDownload(dl)

	dir := filepath.Dir("./download/data.md")

	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}

	f, _ := os.Create("./download/data.md")
	defer f.Close()
	f.Write(b)
}

func main() {
	rootpath := "./download" // dst dir path
	ExportType = "md"        // export type options: pdf、jpg、docx、md
	shimo_sid = shimo_sid    // 石墨cookie内的shimo_sid值

	root := &DirInfo{
		FileInfo: FileInfo{
			Path:  rootpath,
			Id:    "",
			Title: "",
			Type:  "root",
		},
		Dirs:  nil,
		Files: nil,
	}

	StructTree(root)
	TraverseTree(root)

}
