package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	//"github.com/robertkrimen/otto"
	"log"
	"math/rand"
	//"regexp"
	"golang.org/x/net/proxy"
	"net/http"
	"time"
	//"unicode/utf8"
	"github.com/axgle/mahonia"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
)

const (
	USE_PROXY bool = true ///< 是否启用代理
	BASE_URL       = `https://t66y.com/`
)

/**
* 采集内容
 */
type Content struct {
	title       string   ///< 标题
	content_url string   ///< 内容入口地址
	image_urls  []string ///< 缩略图地址
}

/// 区域随机整型数字
func random_int(min, max int) int {
	randNum := rand.Intn(max-min) + min
	return randNum
}

/// 生成随机ip
func random_ip() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		random_int(1, 255), random_int(1, 255), random_int(1, 255), random_int(1, 255))
}

// 判读文件夹是否存在
func isExist(dir string) bool {
	_, err := os.Stat(dir)
	if err == nil {
		return true
	}
	return os.IsExist(err)
}

/// 转换字符串编码
func convertToString(src string, srcCode string, tagCode string) string {
	srcCoder := mahonia.NewDecoder(srcCode)
	srcResult := srcCoder.ConvertString(src)
	tagCoder := mahonia.NewDecoder(tagCode)
	_, cdata, _ := tagCoder.Translate([]byte(srcResult), true)
	result := string(cdata)
	return result
}

//根据url 创建http 请求的 request
//网站有反爬虫策略 wireshark 不解释
func buildRequest(url string) *http.Request {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Accept-Language", `zh-CN,zh;q=0.9`)
	req.Header.Set("User-Agent", `Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.106 Safari/537.36`)
	req.Header.Set("X-Forwarded-For", random_ip())
	req.Header.Set("referer", BASE_URL)

	return req
}

/// 获取远端服务器的HTML页面
func getHtml(url string, use_proxy bool) (*goquery.Document, error) {
	if use_proxy {
		dialSocksProxy, err := proxy.SOCKS5("tcp", "localhost:1080", nil, proxy.Direct)

		tr := &http.Transport{Dial: dialSocksProxy.Dial}

		client := &http.Client{
			Transport: tr,
			Timeout:   time.Second * 5,
		}

		req := buildRequest(url)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}

		return goquery.NewDocumentFromResponse(resp)
	} else {

		client := &http.Client{
			Timeout: time.Second * 5,
		}

		req := buildRequest(url)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			return nil, err
		}

		return goquery.NewDocumentFromResponse(resp)
	}
}

/// 下载文件
func downloadFile(url string, fileName string, c chan int) {
	req := buildRequest(url)
	http.DefaultClient.Timeout = 10 * time.Second
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("failed download ")
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Println("failed download " + url)
		return
	}

	defer func() {
		resp.Body.Close()
		if r := recover(); r != nil {
			fmt.Println(r)
		}
		c <- 0
	}()

	localFile, _ := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0777)

	if _, err := io.Copy(localFile, resp.Body); err != nil {
		panic("failed save " + fileName)
	}

	fmt.Println("success download " + fileName)
}

/// 下载页面采集内容
func downloadContent(content Content, c chan int) {
	fmt.Println("begin download " + content.title)

	base_path := "./1024/" + content.title + "/"
	base_path = strings.Replace(base_path, "\n", "", -1)
	base_path = strings.Replace(base_path, " ", "", -1)

	c1 := make(chan int)
	chanCount := 0

	// 创建目录
	if !isExist(base_path) {
		os.MkdirAll(base_path, 0777)
	}

	// 下载图
	for i := 0; i < len(content.image_urls); i++ {
		img_file := fmt.Sprintf("%s%d%s", base_path, i, path.Ext(content.image_urls[i]))
		fmt.Println(img_file)
		if !isExist(img_file) {
			chanCount += 1
			go downloadFile(content.image_urls[i], img_file, c1)
		}
	}

	for i := 0; i < chanCount; i++ {
		<-c1
	}
	c <- 0
}

/// 获取远端服务器的内容页面
func getContent(url string, content *Content) {

	// 获取远程服务器的页面
	doc, err := getHtml(url, USE_PROXY)
	if err != nil {
		log.Fatal(err)
	}

	// 图片
	doc.Find("input").Each(func(index int, item *goquery.Selection) {
		linkTag := item
		link, _ := linkTag.Attr("src")
		if link != "" {
			content.image_urls = append(content.image_urls, link)
			//fmt.Println(link)
		}
	})

	// 标题
	t := doc.Find("h4")
	title := t.Text()
	title = strings.TrimSpace(title)
	title = convertToString(title, "gbk", "utf8")
	content.title = title
	//fmt.Println(title)
}

/// 获取远端服务器的列表页面
func getPage(page_url string, contents *[]Content) {
	// 获取远程服务器的页面
	doc, err := getHtml(page_url, USE_PROXY)
	if err != nil {
		log.Fatal(err)
	}

	var content_urls []string

	// 获取内容页面的访问入口url
	doc.Find("td.tal a").Each(func(index int, item *goquery.Selection) {
		linkTag := item
		link, _ := linkTag.Attr("href")
		if !strings.Contains(link, "read.php") {
			content_urls = append(content_urls, link)
			//fmt.Println(link)
		}
	})

	// 遍历内容页面
	var content Content
	for i := 0; i < len(content_urls); i++ {
		//fmt.Println(content_urls[i])
		content.content_url = content_urls[i]
		getContent(BASE_URL+content_urls[i], &content)
		*contents = append(*contents, content)
	}
}

/// 爬虫
func spider() {
	// 抓取页面
	var contents []Content
	for i := 1; i <= 1; i++ {
		page_url := fmt.Sprintf("https://t66y.com/thread0806.php?fid=8&search=&page=%d", i)
		//fmt.Println(page_url)
		getPage(page_url, &contents)
	}
	//fmt.Println(len(contents))

	// 下载图片
	c := make(chan int)
	for _, s := range contents {
		go downloadContent(s, c)
		fmt.Println("===========================")
		fmt.Println("页面地址:", s.content_url)
		fmt.Println("标题:", s.title)
	}

	for i := 0; i < len(contents); i++ {
		<-c
	}

	fmt.Println("all done!")
}

func testRandomIP() {
	for i := 0; i < 100; i++ {
		fmt.Println(random_ip())
	}
}

func init() {
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(4)
}

func main() {
	spider()
}
