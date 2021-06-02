/*
Copyright © 2020 iiusky sky@03sec.com

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cos_upgrade

import (
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/hashicorp/go-version"
	"github.com/schollz/progressbar/v3"
	"github.com/theckman/yacspin"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

type CosUpgrade struct {
	AppName        string
	CurrentVersion string
	CosBucket      string
	CosLocation    string
	CosFolder      string
	TmpBinFile     string
	Debug          bool
}

func (upgrade *CosUpgrade) Upgrade() string {
	var v VersionStruct

	currentVersion, err := version.NewVersion(upgrade.CurrentVersion)

	versionEndPoint := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s/%s/version.json",
		upgrade.CosBucket, upgrade.CosLocation, upgrade.CosFolder, upgrade.AppName)

	resp, err := resty.New().R().Get(versionEndPoint)
	if err != nil {
		return "[x] 升级失败,无法检查升级,请联系管理员."
	}

	err = json.Unmarshal(resp.Body(), &v)

	if err != nil {
		return "[x] 升级失败,无法解析升级信息,请联系管理员."
	}

	latestVersion, err := version.NewVersion(v.Version)

	if err != nil {
		return "[x] 升级失败,版本信息错误,请联系管理员."
	}

	if !currentVersion.LessThan(latestVersion) {
		return "[x] 未检查到新版本"
	}

	releaseTime := time.Unix(v.ReleaseTime, 0)

	if upgrade.Debug {
		fmt.Printf("当前版本为 %s\r\n最新版本为 %s\r\n最新版本发布时间为 %s \r\n", upgrade.CurrentVersion, v.Version,
			releaseTime.Format("2006-01-02 15:04:05"))
	}

	downloadStatus, sha256Str := upgrade.downloadRelease()

	if downloadStatus {
		if sha256Hash, err := GetSHA256FromFile(upgrade.TmpBinFile); err != nil {
			return "[x] 升级失败,校验文件错误,请联系管理员."
		} else {
			if sha256Hash != sha256Str {
				return "[x] 升级失败,校验文件失败,请联系管理员."
			}

			if err := os.Rename(upgrade.TmpBinFile, upgrade.AppName); err != nil {
				return "[x] 升级失败,文件重命名失败,请联系管理员."
			}

			return "[+] 升级成功,请重新启动程序."
		}

	} else {
		return "[x] 升级失败,请联系管理员."
	}
}

// 下载编译好的指定版本文件
func (upgrade *CosUpgrade) downloadRelease() (bool, string) {
	releaseBinEndPoint := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s/%s/latest/%s_%s_%s/%s",
		upgrade.CosBucket, upgrade.CosLocation, upgrade.CosFolder, upgrade.AppName, upgrade.AppName,
		runtime.GOOS, runtime.GOARCH, upgrade.AppName)

	if "windows" == runtime.GOOS {
		releaseBinEndPoint = releaseBinEndPoint + ".exe"
	}
	releaseBinEndPointSha256 := fmt.Sprintf("%s_%s_%s.sha256", releaseBinEndPoint, runtime.GOOS, runtime.GOARCH)

	if upgrade.Debug {
		fmt.Println("releaseBinEndPoint:", releaseBinEndPoint)
		fmt.Println("releaseBinEndPointSha256:", releaseBinEndPointSha256)
	}

	req, _ := http.NewRequest("GET", releaseBinEndPoint, nil)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		f, _ := os.OpenFile(upgrade.TmpBinFile, os.O_CREATE|os.O_WRONLY, 0744)
		defer f.Close()

		bar := progressbar.DefaultBytes(
			-1,
			"Downloading",
		)
		_, _ = io.Copy(io.MultiWriter(f, bar), resp.Body)

		if r, err := resty.New().R().Get(releaseBinEndPointSha256); err == nil && r.StatusCode() == 200 {
			return true, r.String()
		} else {
			if upgrade.Debug {
				fmt.Println(fmt.Sprintf("请求 %s 发生异常:%v", releaseBinEndPointSha256, err))
			}
			return false, ""
		}
	} else {
		if upgrade.Debug {
			fmt.Println(fmt.Sprintf("请求 %s 返回值为 %s ,退出.", releaseBinEndPoint, resp.Status))
		}
		return false, ""
	}
}

func (upgrade *CosUpgrade) CheckVersion() {
	fmt.Println(fmt.Sprintf("[+] 当前版本为 v%s", upgrade.CurrentVersion))
	var v VersionStruct

	versionEndPoint := fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s/%s/version.json",
		upgrade.CosBucket, upgrade.CosLocation, upgrade.CosFolder, upgrade.AppName)

	cfg := yacspin.Config{
		Colors:            []string{"fgGreen"},
		Frequency:         75 * time.Millisecond,
		CharSet:           yacspin.CharSets[11],
		Suffix:            "Info",
		SuffixAutoColon:   true,
		Message:           "正在获取服务端最新版本信息.",
		StopCharacter:     "[+]",
		StopFailColors:    []string{"fgRed"},
		StopFailMessage:   "获取失败,无法获取远程最新版本信息,请联系管理员.",
		StopFailCharacter: "[x]",
	}

	spinner, err := yacspin.New(cfg)

	spinner.Start()

	resp, err := resty.New().R().Get(versionEndPoint)
	if err != nil {
		spinner.StopFail()
		if upgrade.Debug {
			fmt.Println(fmt.Sprintf("获取远程最新版本错误信息为:%s ", err))
		}
		return
	}
	spinner.Message("[+] 信息获取成功.")
	spinner.Stop()

	err = json.Unmarshal(resp.Body(), &v)

	if err != nil {
		fmt.Println("[x] 获取失败,无法解析远程最新版本信息,请联系管理员.")
		if upgrade.Debug {
			fmt.Println(fmt.Sprintf("解析最新版本错误信息为:%s ,原始数据为:%s", err, string(resp.Body())))
		}
		return
	}

	releaseTime := time.Unix(v.ReleaseTime, 0)

	fmt.Println(fmt.Sprintf("最新版本为 %s\r\n最新版本发布时间为 %s", v.Version, releaseTime.Format("2006-01-02 15:04:05")))
}
