# Simple Network State Checker

A simple service that allows condensing several different connectivity checks into one status code.
It was originally written to for use with MikroTik's Netwatch service.

Sometimes simple ICMP echo requests and HTTP(S) requests via IP are not enough to determine network connectivity state.

---
If you are not running milti-WANs, complex load-balancers or other stateful network routing setups - you probably don't need this thing.

## Main config file (`check.json`)

```jsonc
{
    "use_cached_results": true, // Use cached test results, or perform tests on-demand?
    "code_healthy": 202, // Respond with this HTTP code, when the cluster is OK
    "code_degraded": 406, // Respond with this HTTP code, when the cluster is Degraded
    "code_failed": 500, // Respond with this HTTP code, when the cluster is Failed
    "servers": [
        {
            "type": "http", // Type of check
            "display_name":"Google Resolve", // Pretty name for dashboards
            "check_period_seconds": 5, // How often the batch of tests is ran (integer)
            "url": "https://google.com", // Target URL
            "follow_redir": false, // Determines whether the HTTP client would follow redirects (301, 302, etc)
            "success_codes": [301], // Which HTTP codes are considered OK
            "check_code": true, // Whether or not to check HTTP code against the array above
            "test_count": 3, // How many tests would be in each batch
            "test_delay_ms": 800, // ms between tests in each batch (integer)
            "timeout_ms":1000, // ms, TCP timeout
            "critical": true // If true, any hiccup would throw entire cluster into FAILED state
        }
    ]
}
```

## Environment setup and SSL
You can pass `PORT` env var. If the port is `443`, the service would look for files named `cert.crt` and `priv.key` (named exactly) in the immediate vicinity of the executable file to serve content with SSL.

## Build process
**Ensure the go build toolchain is set up and is working correctly before proceeding!**
```bash
git clone https://github.com/Alex-Dash/simple-network-state-checker
cd simple-network-state-checker
go get
go build
```

## Example systemd service
```conf
[Unit]
Description=Simple Network State Checker
After=network.target

[Service]
ExecStart=/path/to/executable/snsc <-- CHANGE THIS
Type=simple
Restart=on-failure


[Install]
WantedBy=default.target
RequiredBy=network.target
```

Save the above into `snsc.service` and put it into `/etc/systemd/system/snsc.service`

Then run `sudo systemctl daemon-reload` and `sudo systemctl enable snsc`

You may now reboot or start your serivce manually