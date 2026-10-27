(function () {
  var key = null; // solo en memoria: nunca se persiste
  var form = document.getElementById('gate');
  var err = document.getElementById('err');
  form.addEventListener('submit', function (e) {
    e.preventDefault();
    key = document.getElementById('key').value.trim();
    err.textContent = '';
    fetch('/integration/v1/openapi.json', { headers: { 'X-Api-Key': key } })
      .then(function (r) { if (!r.ok) throw new Error(r.status === 403 ? 'La API key no está habilitada.' : 'API key inválida.'); return r.json(); })
      .then(function (spec) {
        form.style.display = 'none';
        document.getElementById('ui').style.display = 'block';
        window.SwaggerUIBundle({
          spec: spec, dom_id: '#ui', deepLinking: false, persistAuthorization: false,
          requestInterceptor: function (req) { req.headers['X-Api-Key'] = key; return req; }
        });
      })
      .catch(function (e) { err.textContent = e.message; });
  });
})();
