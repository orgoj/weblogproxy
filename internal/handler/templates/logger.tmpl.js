{{/*
This is the Go template for the dynamic logger.js script.
It receives LoggerJsData as input.
Go template comments like this one are not rendered in the output.
*/}}
(function() {
    'use strict';

    const config = {
        logEnabled: {{.LogEnabled}},
        siteId: "{{.SiteID}}",
        gtmId: "{{.GtmID}}",
        token: "{{.Token}}",
        logUrl: "{{.LogURL}}",
        scriptsToInject: [
            {{range .ScriptsToInject}}
            {
                url: "{{.URL}}",
                async: {{.Async}},
                defer: {{.Defer}}
            },
            {{end}}
        ]
    };

    function sendLog(data) {
        if (!config.logEnabled) {
            return;
        }

        const payload = {
            token: config.token,
            site_id: config.siteId,
            gtm_id: config.gtmId,
            data: {}
        };

        if (typeof data === 'object' && data !== null) {
            payload.data = data;
        } else {
            payload.data = { message: String(data) };
        }

        navigator.sendBeacon(config.logUrl, JSON.stringify(payload));
    }

    function injectScript(script) {
        const scriptElement = document.createElement('script');
        scriptElement.src = script.url;
        if (script.async) {
            scriptElement.async = true;
        }
        if (script.defer) {
            scriptElement.defer = true;
        }
        (document.head || document.body || document.documentElement).appendChild(scriptElement);
    }

    window.weblogproxy = window.weblogproxy || {};
    window.weblogproxy.log = sendLog;
    window.weblogproxy.config = config;

    if (config.scriptsToInject && config.scriptsToInject.length > 0) {
        config.scriptsToInject.forEach(injectScript);
    }
})(); 