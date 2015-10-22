Simple, but thorough media checker.

Give it a URL and it will fetch it, extract all links to
CSS/JS/Images, etc on the page and check that each exists. It will
fail if any are not fetchable, if any take more than a specified
timeout to fetch, if any of them have SSL issues, or if there are
HTTP/HTTPS mixed content issues.


## Usage


```
$ ./mediacheck --help
Usage of ./mediacheck:
  -log-format string
        log format: text or json (default "text")
  -log-level string
        log level: info/warn/error (default "info")
  -timeout int
        timeout (ms) (default 3000)
  -url string
        URL to check
```

The only required flag is `-url`. `-timeout` is handy if you want to
enforce faster media loading or if you want to allow slower media
loading.

`-log-level` controls verbosity. `-log-format` lets you output the
results as JSON for easier parsing. (uses
[logrus](https://github.com/Sirupsen/logrus) behind the scenes).

## Examples

A successful check:

```
$ ./mediacheck -url=https://www.google.com/
INFO[0000] fetching                                      Host=www.google.com Path=/ Scheme=https URL=https://www.google.com/
INFO[0000] retrieved page                               
INFO[0000] extracted media URLs                          number=3
INFO[0000] checking media URL                            url=https://www.google.com/images/branding/googlelogo/1x/googlelogo_white_background_color_272x92dp.png
INFO[0000] checking media URL                            url=https://www.google.com/images/branding/product/ico/googleg_lodp.ico
INFO[0000] checking media URL                            url=https://www.google.com/images/icons/product/chrome-48.png
INFO[0000] OK                                           
```

This check fails on a few resources:

```
./mediacheck -url=http://welcome.ccnmtl.columbia.edu/ -log-level=error
ERRO[0000] not a 200                                     status=403 Forbidden url=http://welcome.ccnmtl.columbia.edu/files/custom-css/custom-css-1425436612.min.css?ver=4.1.8
ERRO[0001] Error fetching media                          Error=bad status URL=http://welcome.ccnmtl.columbia.edu/files/custom-css/custom-css-1425436612.min.css?ver=4.1.8
FATA[0001] NOT OK                                       
```

A linked CSS file is returning a 403.
