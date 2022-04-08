/*
Copyright 2022 The KodeRover Authors.

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

package gitee

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"gitee.com/openeuler/go-gitee/gitee"
	"golang.org/x/oauth2"

	"github.com/koderover/zadig/pkg/shared/client/systemconfig"
)

type Client struct {
	*gitee.APIClient
}

func NewClient(id int, accessToken, proxyAddr string, enableProxy bool) *Client {
	var (
		client     *gitee.APIClient
		HttpClient *http.Client
	)

	conf := gitee.NewConfiguration()
	dc := http.DefaultClient
	if enableProxy {
		p, err := url.Parse(proxyAddr)
		if err == nil {
			proxy := http.ProxyURL(p)
			trans := &http.Transport{
				Proxy: proxy,
			}
			dc = &http.Client{Transport: trans}
		}
	}

	if accessToken != "" {
		ch, err := systemconfig.New().GetCodeHost(id)
		// The normal expiration time is 86400
		if err == nil && (time.Now().Unix()-ch.UpdatedAt) >= 86000 {
			token, err := RefreshAccessToken(ch.RefreshToken)
			if err == nil {
				accessToken = token.AccessToken
				ch.AccessToken = token.AccessToken
				ch.RefreshToken = token.RefreshToken
				ch.UpdatedAt = int64(token.CreatedAt)

				if err = systemconfig.New().UpdateCodeHost(ch.ID, ch); err != nil {
					fmt.Println(fmt.Sprintf("failed to updateCodeHost err:%s", err))
				}
			}
		}

		ctx := context.WithValue(context.Background(), oauth2.HTTPClient, dc)
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: accessToken},
		)
		HttpClient = oauth2.NewClient(ctx, ts)
	} else {
		HttpClient = dc
	}
	conf.HTTPClient = HttpClient
	client = gitee.NewAPIClient(conf)

	return &Client{client}
}
