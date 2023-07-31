package controller

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
)

func (c *D3osGatewayController) Page404Handler(w http.ResponseWriter, r *http.Request) {
	str := fmt.Sprintf("%s/%s,404.not.found", r.URL.Host, r.URL.Path)
	if _, ok := w.Header()["Content-Length"]; ok {
		w.Header().Set("Content-Length", strconv.Itoa(len(str)))
	} else {
		w.Header().Add("Content-Length", strconv.Itoa(len(str)))
	}
	if _, ok := w.Header()["Content-Type"]; ok {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else {
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(404)
	_, _ = w.Write([]byte(str))
}

func (c *D3osGatewayController) HandleRequestAndRedirect(res http.ResponseWriter, req *http.Request) {
	fullUrl := req.Host + req.URL.Path
	klog.Infof("client_request[host:%v][method:%v][remote_addr:%v][Path:%v][fullUrl:%v][req.RequestURI:%v]",
		req.Host,
		req.Method,
		req.RemoteAddr,
		req.URL.Path,
		fullUrl,
		req.RequestURI,
	)

	targetV := ""
	compairStr := fmt.Sprintf("%s/%s", strings.Split(req.Host, ":")[0], req.URL.Path)
	value, ok := c.ingressMap.Load(compairStr)
	if ok {
		klog.Infof("在ingressMap中找到key: %s, value: %s", compairStr, value)
		targetV = value.(string)
	}
	if targetV == "" {
		//返回404
		klog.Infof("fullUrl.ingressMap.not.found.404:%v", fullUrl)
		c.Page404Handler(res, req)
		return
	}

	// svcName + ns:port
	svcStr := targetV
	klog.Infof("fullUrl.foundsvc.print[fullUrl:%v][svcStr:%v]", fullUrl, svcStr)

	c.ServeReverseProxy(svcStr, res, req)
}

func (c *D3osGatewayController) ServeReverseProxy(target string, res http.ResponseWriter, req *http.Request) {
	// parse the url
	urlT, _ := url.Parse("http://" + target)

	// create the reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(urlT)

	// Update the headers to allow for SSL redirection
	req.URL.Host = urlT.Host
	req.URL.Scheme = urlT.Scheme
	req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
	req.Host = urlT.Host

	// Note that ServeHttp is non blocking and uses a go routine under the hood
	proxy.ServeHTTP(res, req)
}
