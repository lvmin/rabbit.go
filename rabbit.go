// @_@! shajj1983
// 2015-01-09 22:17
package rabbit

import (
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"fmt"
	"crypto/md5"
	"encoding/hex"
	"os"
	"encoding/json"
	"runtime"

	_ "github.com/go-sql-driver/mysql"
)

const (
	author = "shajj1983"
	version = "0.9.1-dev"
)

const (
	TemplateDir = "template"                                                    //模板路径
)

var (
	Template = make(map[string]*template.Template)
)

//故障监控标签
var (
	healthTag = true
)

func init() {

}

func RabbitCoreErrorCheck(err error){
	if err != nil{
		healthTag = false
		panic(err)
	}
}

type RabbitError struct {
	LineNumber int32
	FileName string
	Msg string
}

func (e *RabbitError) Error() string {
	return fmt.Sprintf("%s-%d-%s",e.FileName,e.LineNumber,e.Msg)
}

type Rabbit struct {
	W   http.ResponseWriter
	R   *http.Request
	HealthTag bool
	GET map[string]string
}

func (this *Rabbit) Output(msg string) {
	layout := `<html>
<head><title>`+ msg +`</title></head>
<body bgcolor="white">
<center><h1>` + msg + `</h1></center>
<hr><center>golang bear `+version+`</center>
</body>
</html>`

	this.writeString(layout)
}

func (this *Rabbit) Abort (httpCode int) {

	if httpCode == http.StatusInternalServerError {
		this.W.WriteHeader(http.StatusInternalServerError)

		this.Output("500 Internal Server Error")
		//http.Error(rabbit.W, e.Error(), http.StatusInternalServerError)
	}

}

func (this *Rabbit) Log(msg string){
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	log.Println(CurrentStateInfo(2) + msg)
}

func (this *Rabbit) Debug(msg string) {
	this.writeString(msg)
}


func (this *Rabbit) Redirect(target string) {
	http.Redirect(this.W, this.R, target, http.StatusFound)
}

func (this *Rabbit) RenderMulti(s map[string]interface{}) {

	tplFiles := GetArrayOfkeyFromMap(s)

	var tpls = make([]string,len(tplFiles))
	copy(tpls,tplFiles)

	for i,tplFileName := range tplFiles{
		tplFiles[i] = TemplateDir + "/" + tplFileName + ".html"
	}

	resource,err := template.ParseFiles(tplFiles...)
	RabbitCoreErrorCheck(err)

	for _,tpl := range tpls{
		resource.ExecuteTemplate(this.W, tpl, s[tpl])
	}

}

func (this *Rabbit) Render(tpl string,args map[string]interface{}){

	// template cache
	if _,ok := Template[tpl]; ok {
		Template[tpl].Execute(this.W,args)
		return
	}

	resource, err := template.ParseFiles(TemplateDir + "/" + tpl + ".html")
	RabbitCoreErrorCheck(err)
	resource.Execute(this.W, args)
}

func (this *Rabbit) JsonOutput(jsonData string) {
	//this.W.Header().Set("content-type", "application/json")
	this.writeString(jsonData)
}

func (this *Rabbit) writeString(msg string) {
	io.WriteString(this.W, msg)
}

func (this *Rabbit) ParseQueryUrl(regular string) map[string]string {
	rep := regexp.MustCompile(regular)
	box := make(map[string]string)

	result := rep.FindStringSubmatch(this.R.RequestURI)
	if result == nil {
		return box
	}
	for i, key := range rep.SubexpNames() {
		if i == 0 || key == "" {
			continue
		}
		box[key] = result[i]
	}
	return box
}

var (
	mountHandle  = make(map[string]func(*Rabbit))
	routerHandle = make(map[string]func(*Rabbit))
)

func strongHandle(fn func(*Rabbit)) func(*Rabbit) {
	return func(rabbit *Rabbit) {
		defer func() {
			if e, ok := recover().(error); ok {
				rabbit.W.WriteHeader(http.StatusInternalServerError)
				rabbit.Output("500 Internal Server Error")
				//http.Error(rabbit.W, e.Error(), http.StatusInternalServerError)
				log.Println("WARN:panic in ", CurrentStateInfo(5), e.Error())
			}
		}()
		fn(rabbit)
	}
}

func Phoenix(fn func(args ...interface{}),args ...interface{}) {
	func() {
		defer func() {
			if e, ok := recover().(error); ok {
				log.Println("WARN:panic in ", CurrentStateInfo(5), e.Error())
			}
		}()
		fn(args...)
	}()
}

func Mount(way string, fn func(*Rabbit)) {

	mountHandle[way] = strongHandle(fn)
}

type Mux struct {
}

func (this *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//defer DbObject.Close() //存在请求一次就 db close情况关闭数据库连接
	defer r.Body.Close()
	
	rabbit := &Rabbit{}

	for queryUri, mountFunc := range mountHandle {
		if ok, _ := regexp.MatchString(queryUri, r.RequestURI); ok {
			
			rabbit.W = w
			rabbit.R = r
			rabbit.HealthTag = healthTag
			rabbit.GET = rabbit.ParseQueryUrl(queryUri)
			mountFunc(rabbit)
			return
		}
	}

	rabbit.W = w
	rabbit.R = r
	rabbit.HealthTag = healthTag
	//http.NotFound(w, r)
	w.WriteHeader(http.StatusNotFound)
	rabbit.Output("404 Not Found")
	return
}

func Run(opt string) {

	runtime.GOMAXPROCS(runtime.NumCPU())

	err := http.ListenAndServe(opt, &Mux{})
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func Router(way string, fn func(*Rabbit)) {
	routerHandle[way] = strongHandle(fn)
}

func GO(rabbit *Rabbit) {
	for queryUri, routerFunc := range routerHandle {
		if ok, _ := regexp.MatchString(queryUri, rabbit.R.RequestURI); ok {
			routerFunc(rabbit)
		}
	}
}

var (
	DB *BaseDb
	// DbObject *sql.DB
)

type BaseDb struct {
	db *sql.DB
	Dsn string
}

func (this *BaseDb) Register(dsn string) {
	DB = new(BaseDb)
	DB.Dsn = dsn
}

func Db() (*BaseDb,*sql.DB) {

	db, err := sql.Open("mysql", DB.Dsn)
	// db.SetMaxOpenConns(10000)
	// db.SetMaxIdleConns(1000)
	if err != nil {
		defer db.Close()
		RabbitCoreErrorCheck(err)
	}
	// DB.db = db
	// DbObject = db
	return DB,db
}

func (this *BaseDb) Query(db *sql.DB,sqlstr string) (int64, error) {
	stmt, err := db.Prepare(sqlstr)
	RabbitCoreErrorCheck(err)
	defer stmt.Close()

	result, err := stmt.Exec()
	if err != nil {
		RabbitCoreErrorCheck(err)
	}
	return result.LastInsertId()
}

func (this *BaseDb) Fetch(db *sql.DB,sqlstr string) ([]map[string]string, error) {
	stmt, err := db.Prepare(sqlstr)
	if err != nil {
		RabbitCoreErrorCheck(err)
	}
	defer stmt.Close()

	rows, err := stmt.Query()
	RabbitCoreErrorCheck(err)

	columns, err := rows.Columns()
	RabbitCoreErrorCheck(err)

	values := make([]sql.RawBytes, len(columns))
	scanArgs := make([]interface{}, len(values))

	ret := make([]map[string]string, 0)
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		err = rows.Scan(scanArgs...)
		RabbitCoreErrorCheck(err)
		var value string
		vmap := make(map[string]string, len(scanArgs))
		for i, col := range values {
			if col == nil {
				value = "NULL"
			} else {
				value = string(col)
			}
			vmap[columns[i]] = value
		}
		ret = append(ret, vmap)
	}
	return ret, nil	
}

func (this *BaseDb) GetOne(db *sql.DB,sqlstr string) (map[string]string, error) {
	stmt, err := db.Prepare(sqlstr)

	RabbitCoreErrorCheck(err)
	defer stmt.Close()

	rows, err := stmt.Query()
	RabbitCoreErrorCheck(err)

	columns, err := rows.Columns()
	RabbitCoreErrorCheck(err)

	values := make([]sql.RawBytes, len(columns))
	scanArgs := make([]interface{}, len(values))

	ret := make(map[string]string, 0)
	for i := range values {
		scanArgs[i] = &values[i]
	}

	if rows.Next() {
		err = rows.Scan(scanArgs...)
		RabbitCoreErrorCheck(err)
		var value string
		vmap := make(map[string]string, len(scanArgs))
		for i, col := range values {
			if col == nil {
				value = "NULL"
			} else {
				value = string(col)
			}
			vmap[columns[i]] = value
		}
		ret = vmap
	}
	return ret, nil	
}

func (this *BaseDb) GetCount(db *sql.DB,sqlstr string) (int64, error) {
	stmt, err := db.Prepare(sqlstr)

	if err != nil {
		RabbitCoreErrorCheck(err)
	}
	defer stmt.Close()

	rows, err := stmt.Query()
	RabbitCoreErrorCheck(err)

	var id int64

	if rows.Next() {
		err = rows.Scan(&id)
		RabbitCoreErrorCheck(err)
	}
	return id, nil	
}

func (this *BaseDb) Close(db *sql.DB) {
	db.Close() 
}


//commom function

func DebugConsole(str string) {
	fmt.Print("%v",str)
}

func GetArrayOfkeyFromMap(values map[string]interface{}) []string{
	keys := make([]string, len(values))

	var i = 0
	for k := range values {
	    keys[i] = k
	    i++
	}
	return keys
}

func GetkeyValueFromMap(values map[string]interface{}) (string,interface{},error){
	var (
		key string
		value interface{}
	)
	for k,v := range values {
	    key,value = k,v
	}
	return key,value,nil
}

func InArray(needle string,haystack []string) bool{
	for _,v := range haystack{
		if v == needle{
			return true
		}
	}
	return false
}

func Md5(s string) string {
    h := md5.New()
    h.Write([]byte(s))
    return hex.EncodeToString(h.Sum(nil))
}

func FileExist(filename string) bool {
    _, err := os.Stat(filename)
    return err == nil || os.IsExist(err)
}

func JsonEncode(jsonData interface{}) string{
	result,err := json.Marshal(jsonData);
	if err != nil {
    	RabbitCoreErrorCheck(err)
	}
	return string(result)
}

func CurrentStateInfo(level int) string{
	var currentFileInfo string
	funcName, file, line, ok := runtime.Caller(level)
	if ok {
	    currentFileInfo = fmt.Sprintf("(%s-%s-%d)",file,runtime.FuncForPC(funcName).Name(),line)
	}
	return currentFileInfo
}
