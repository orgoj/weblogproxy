{{/*
This is the Go template for the dynamic logger.js script.
It receives LoggerJsData as input.
Go template comments like this one are not rendered in the output.
*/}}(function() {
    'use strict';
    {{if .LogEnabled}}
    const config = {
        logEnabled: true,
        siteId: "{{.SiteID}}",
        gtmId: "{{.GtmID}}",
        token: "{{.Token}}",
        logUrl: "{{.LogURL}}"
        {{if .ScriptsToInject}}
        ,scriptsToInject: [
            {{range .ScriptsToInject}}
            {
                url: "{{.URL}}",
                async: {{.Async}},
                defer: {{.Defer}}
            },
            {{end}}
        ]
        {{end}}
    };

    function sendLog(data) {
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

    {{if .ScriptsToInject}}
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

    if (config.scriptsToInject && config.scriptsToInject.length > 0) {
        config.scriptsToInject.forEach(injectScript);
    }
    {{end}}

    window.{{.GlobalObjectName}} = window.{{.GlobalObjectName}} || {};
    window.{{.GlobalObjectName}}.log = sendLog;
    window.{{.GlobalObjectName}}.config = config;
    {{else}}
    window.{{.GlobalObjectName}} = window.{{.GlobalObjectName}} || {};
    window.{{.GlobalObjectName}}.log = function() {};
    {{end}}
})();