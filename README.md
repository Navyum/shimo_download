# shimo_download
* 功能：从石墨文档全量导出文件到本地，支持导出格式md、docx、pdf、jpg
* 支持：定义本地路径，需设置rootpath
  ```
     	rootpath := "./download"    // dst dir path ,定义导出路径
  ```
* 支持：设置导出格式为,定义ExportType值，可选：pdf、jpg、docx、md，默认：md
  ```
  	ExportType = "md"        // export type options: pdf、jpg、docx、md
  ```
* 使用必须需要设置shimo_sid:
  ```
	shimo_sid = shimo_sid    // 石墨cookie内的shimo_sid值
  ```
  浏览器F12——>找到导航栏 Application-> 选中左侧 Cookies-> 点击选中 https:shimo.im->点击右侧 shimo_sid->复制value
  <img width="1578" alt="image" src="https://github.com/Navyum/shimo_download/assets/36869790/9af9f5f0-65ec-4452-b863-b90da9c30281">

