package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// global config
var exportType string = "md"
var SleepTime time.Duration = 10 * time.Second
var Retry10 int = 10
var Retry2 int = 2
var Retry20 int = 20
var shimoSid string = ""

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

// httpRequest 发起HTTP请求并处理错误
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
		Value: shimoSid,
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

// handleError 处理错误并返回默认值
func handleError(err error, defaultValue []byte) ([]byte, error) {
	fmt.Println(err)
	return defaultValue, err
}

// 获取文件夹结构信息
func fetchDirectoryInfo(path string, id string, d *DirList, f *FileList) {
	uri := ROOT_URL
	if id != "" {
		uri = fmt.Sprintf(LIST_URL, id)
	}

	b, err := httpRequest(uri, Retry2)
	if err != nil {
		panic(err)
	}

	var result FileResponse
	if err := json.Unmarshal(b, &result); err != nil {
		panic(err)
	}

	for _, fileInfo := range result {
		switch fileInfo.Type {
		case "folder":
			*d = append(*d, createDirInfo(path, fileInfo))
		case "newdoc":
			*f = append(*f, createFileInfo(path, fileInfo))
		}
	}
}

// createDirInfo 创建目录信息
func createDirInfo(path string, fileInfo FileInfo) DirInfo {
	return DirInfo{
		FileInfo: FileInfo{
			Path:  path + "/" + fileInfo.Title,
			Id:    fileInfo.Id,
			Title: fileInfo.Title,
			Type:  fileInfo.Type,
		},
		Dirs:  nil,
		Files: nil,
	}
}

// createFileInfo 创建文件信息
func createFileInfo(path string, fileInfo FileInfo) FileInfo {
	return FileInfo{
		Path:  path + "/" + fileInfo.Title,
		Id:    fileInfo.Id,
		Title: fileInfo.Title,
		Type:  fileInfo.Type,
	}
}

// 获取导出文件,默认按照md格式
func httpExport(id string) (tid string) {
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

// checkShimoSid 检查shimoSid是否有效
func checkShimoSid() error {
	uri := ROOT_URL
	resp, err := httpRequest(uri, Retry2)
	if err != nil {
		return err
	}

	// Assuming a 200 status code means the shimoSid is valid
	if string(resp) == "http request error occur" {
		return errors.New("invalid shimoSid")
	}

	return nil
}

// 递归构造文件结构树,基于深度优先
func buildStructureTree(tree *DirInfo, fileCount *int, logger Logger) {
	dirs := &DirList{}
	files := &FileList{}

	fetchDirectoryInfo(tree.Path, tree.Id, dirs, files)

	tree.Files = files
	tree.Dirs = dirs

	for _, file := range *files {
		logger.Log(fmt.Sprintf("[文件结构]: %s", file.Path))
		*fileCount++
	}

	for i := range *dirs {
		buildStructureTree(&(*dirs)[i], fileCount, logger)
	}
}

// 遍历文件结构
func TraverseTree(tree *DirInfo, removeBlank bool, logger Logger) {
	node := *tree
	if node.Files != nil {
		fl := *(node.Files)
		for i := range fl {
			fmt.Println("-------------------")
			logger.Log(fmt.Sprintf("[TraverseTree]: %s", fl[i]))
			tid := httpExport(fl[i].Id)
			(*(node.Files))[i].TaskId = tid

			DiskDownload(fl[i], removeBlank)
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
		TraverseTree(&dl[i], removeBlank, logger)
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

func RemoveBlank(path string) string {
	fmt.Println("【移除空格】: ", path)
	return strings.ReplaceAll(path, " ", "")
}

// 将f下载到磁盘
func DiskDownload(f FileInfo, removeBlank bool) {
	dl := httpLinkQuery(f.TaskId)
	b := httpDownload(dl)

	dir := filepath.Dir(f.Path)

	if removeBlank {
		dir = RemoveBlank(dir)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}

	path := duplicateTitle(f.Path)
	if removeBlank {
		path = RemoveBlank(path)
	}

	fmt.Println(path)

	file, err := os.Create(path + "." + exportType)
	if err != nil {
		fmt.Println(err)
	}
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

// 定义日志接口
type Logger interface {
	Log(message string)
}

// 实现Logger接口的结构体
type AppLogger struct {
	logOutput          *widget.RichText
	logOutputContainer *container.Scroll
}

// Log方法实现
func (l *AppLogger) Log(message string) {
	l.logOutput.Segments = append(l.logOutput.Segments, &widget.TextSegment{
		Text: message,
	})
	//l.logOutput.SetText(l.logOutput.Text + message + "\n")
	l.logOutput.Refresh()
	l.logOutputContainer.ScrollToBottom()
}

func main() {

	os.MkdirAll("./abc", 0755)
	var rootpath string
	var removeBlank bool

	myApp := app.New()
	myWindow := myApp.NewWindow("石墨文档导出工具")

	myApp.Settings().SetTheme(theme.DarkTheme()) // 设置为黑色主题

	// 创建输入框
	rootpathEntry := widget.NewEntry()
	rootpathEntry.SetPlaceHolder("请输入目标目录路径,需要自行创建")

	// 如果输入框为空，则设置为默认值
	if rootpathEntry.Text == "" {
		rootpathEntry.SetText("./download")
	}

	// 创建导出类型下拉选择框
	exportTypeSelect := widget.NewSelect([]string{"md", "pdf", "jpg", "docx"}, func(selected string) {
		// 处理选择逻辑
		exportType = selected
	})

	// 仅当没有选择时，设置默认值为md
	if exportTypeSelect.Selected == "" {
		exportTypeSelect.SetSelected("md") // 设置默认值为md
		exportType = "md"
	}

	shimoSidEntry := widget.NewEntry()
	shimoSidEntry.SetPlaceHolder("请输入shimo_sid")

	removeBlankCheck := widget.NewCheck("移除文件名中的空格", func(checked bool) {
		// 处理复选框逻辑
		removeBlank = checked
	})

	removeBlankCheck.Checked = true
	removeBlank = removeBlankCheck.Checked

	// 创建日志输出框
	logOutput := widget.NewRichText()
	//logOutput.SetPlaceHolder("日志输出...")

	//logOutput.SetReadOnly(true)
	logOutputContainer := container.NewScroll(logOutput)
	logOutputContainer.SetMinSize(fyne.NewSize(0, 400))

	// 创建开始下载按钮
	startButton := widget.NewButton("开始下载", func() {
		logOutputContainer.ScrollToBottom()
	})

	logger := &AppLogger{
		logOutput:          logOutput,
		logOutputContainer: logOutputContainer,
	}

	startButton.Importance = widget.HighImportance
	startButton.OnTapped = func() {
		// 这里可以添加处理下载逻辑的代码
		rootpath = rootpathEntry.Text
		shimoSid = shimoSidEntry.Text
		//创建路径为rootpath的文件夹
		os.MkdirAll(rootpath, 0755)

		//检查shimo_sid是否设置
		if shimoSid == "" {
			logger.Log("【错误】: shimo_sid未设置")
			dialog.ShowError(errors.New("【shimo_sid】 未设置"), myWindow)
			return
		}

		// 检查shimoSid是否有效
		err := checkShimoSid()
		if err != nil {
			logger.Log("【错误】: shimo_sid无效")
			dialog.ShowError(errors.New("【shimo_sid】 格式错误或者无效"), myWindow)
			return
		}

		logger.Log(fmt.Sprintf("【下载配置如下：】:\n  -目标路径: %s\n  -导出类型: %s\n  -shimo_sid: %s\n  -移除空格: %v\n", rootpath, exportType, shimoSid, removeBlank))
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

		startButton.Disable()
		startButton.Importance = widget.LowImportance // Change appearance to gray

		startButton.SetText("下载中，请稍后...")

		var fileCount int = 0
		buildStructureTree(root, &fileCount, logger)
		logger.Log(fmt.Sprintf("【查询到文件数量】：%d\n", fileCount))
		TraverseTree(root, removeBlank, logger)

		startButton.Enable()
		startButton.SetText("重新下载")
		startButton.Importance = widget.HighImportance // Change appearance to gray

	}

	// 布局
	myWindow.SetContent(container.NewVBox(
		widget.NewLabel("导出路径:"),
		rootpathEntry,
		widget.NewLabel("导出类型:"),
		exportTypeSelect,
		widget.NewLabel("shimo_sid:"),
		shimoSidEntry,
		removeBlankCheck,
		startButton,
		logger.logOutputContainer,
	))

	myWindow.Resize(fyne.NewSize(400, 600)) // 设置窗口大小
	myWindow.ShowAndRun()

}
