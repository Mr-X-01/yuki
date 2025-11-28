package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"yuki-server/client"

	"github.com/gorilla/mux"
)

type API struct {
	clientManager *client.Manager
	apiKey        string
	adminLogin    string
	adminPassword string
}

func NewAPI(clientManager *client.Manager, apiKey string, adminLogin string, adminPassword string) *API {
	return &API{
		clientManager: clientManager,
		apiKey:        apiKey,
		adminLogin:    adminLogin,
		adminPassword: adminPassword,
	}
}

type CreateClientRequest struct {
	Name         string     `json:"name"`
	MaxBandwidth int64      `json:"max_bandwidth"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type ClientResponse struct {
	*client.Client
	Config string `json:"config,omitempty"`
}


func (a *API) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	client := a.clientManager.CreateClient(req.Name, req.MaxBandwidth, req.ExpiresAt)
	
	// Получаем server address из окружения или используем домен из запроса
	serverAddr := r.Host
	if serverAddr == "" {
		serverAddr = "localhost:8443"
	}
	
	// Generate client config в формате совместимом с клиентом
	config := map[string]interface{}{
		"server_address": serverAddr,
		"client_id":      client.ID,
		"client_secret":  client.Secret,
		"protocol":       "grpc",
		"encryption":     "xchacha20-poly1305",
		"tun_settings": map[string]interface{}{
			"name":    "yuki",
			"ip":      "10.0.0.2",
			"netmask": "255.255.255.0",
			"gateway": "10.0.0.1",
			"dns":     []string{"8.8.8.8", "8.8.4.4"},
		},
		"advanced": map[string]interface{}{
			"keep_alive":  30,
			"reconnect":   true,
			"auto_start":  false,
			"kill_switch": false,
		},
	}

	configJSON, _ := json.Marshal(config)

	response := ClientResponse{
		Client: client,
		Config: string(configJSON),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *API) DeleteClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["uuid"]

	if !a.clientManager.DeleteClient(clientID) {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListClients(w http.ResponseWriter, r *http.Request) {
	clients := a.clientManager.ListClients()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (a *API) BlockClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["uuid"]

	if !a.clientManager.BlockClient(clientID) {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "blocked"}`))
}

func (a *API) UnblockClient(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	clientID := vars["uuid"]

	if !a.clientManager.UnblockClient(clientID) {
		http.Error(w, "Client not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "unblocked"}`))
}

func (a *API) GetStats(w http.ResponseWriter, r *http.Request) {
	clients := a.clientManager.ListClients()
	
	totalClients := len(clients)
	activeClients := 0
	totalTrafficUp := int64(0)
	totalTrafficDown := int64(0)

	for _, client := range clients {
		if client.Active {
			activeClients++
		}
		totalTrafficUp += client.BytesUp
		totalTrafficDown += client.BytesDown
	}

	stats := map[string]interface{}{
		"total_clients":      totalClients,
		"active_clients":     activeClients,
		"total_traffic_up":   totalTrafficUp,
		"total_traffic_down": totalTrafficDown,
		"server_uptime":      time.Since(time.Now().Add(-time.Hour)).Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// Legitimate API endpoints for cover
func (a *API) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"service":   "api-gateway",
		"version":   "1.2.3",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *API) GetSystemStatus(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"cpu_usage":    45.2,
		"memory_usage": 67.8,
		"disk_usage":   34.1,
		"load_avg":     []float64{0.5, 0.3, 0.2},
		"uptime":       86400,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *API) SetupRoutes() *mux.Router {
	router := mux.NewRouter()

	// Public endpoints (no auth needed)
	router.HandleFunc("/health", a.HealthCheck).Methods("GET")
	router.HandleFunc("/api/v1/status", a.GetSystemStatus).Methods("GET")
	router.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	}).Methods("GET")
	router.HandleFunc("/admin/", a.AdminPanelPage).Methods("GET")
	router.HandleFunc("/admin/api/login", a.LoginHandler).Methods("POST")

	// Admin API endpoints (protected by session)
	api := router.PathPrefix("/admin/api").Subrouter()
	api.Use(a.sessionMiddleware)
	api.HandleFunc("/clients", a.CreateClient).Methods("POST")
	api.HandleFunc("/clients", a.ListClients).Methods("GET")
	api.HandleFunc("/clients/{uuid}", a.DeleteClient).Methods("DELETE")
	api.HandleFunc("/clients/{uuid}/block", a.BlockClient).Methods("POST")
	api.HandleFunc("/clients/{uuid}/unblock", a.UnblockClient).Methods("POST")
	api.HandleFunc("/stats", a.GetStats).Methods("GET")

	return router
}

func (a *API) AdminPanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>

<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Yuki Admin Panel</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; padding: 40px 20px; }
		.header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 40px; border-radius: 8px; margin-bottom: 30px; display: flex; justify-content: space-between; align-items: center; }
		.header h1 { font-size: 2.5em; margin-bottom: 10px; }
		.header p { font-size: 1.1em; opacity: 0.9; }
		.header-right { text-align: right; }
		.logout-btn { background: rgba(255,255,255,0.2); color: white; padding: 10px 20px; border: 1px solid white; border-radius: 4px; cursor: pointer; }
		.logout-btn:hover { background: rgba(255,255,255,0.3); }
		.section { background: white; padding: 30px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.section h2 { color: #333; margin-bottom: 20px; border-bottom: 2px solid #667eea; padding-bottom: 10px; }
		.form-group { margin-bottom: 15px; }
		.form-group label { display: block; color: #333; font-weight: 500; margin-bottom: 5px; }
		.form-group input, .form-group select { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 1em; }
		.form-group input:focus, .form-group select:focus { outline: none; border-color: #667eea; box-shadow: 0 0 5px rgba(102, 126, 234, 0.3); }
		.btn { background: #667eea; color: white; padding: 12px 24px; border: none; border-radius: 4px; cursor: pointer; font-size: 1em; font-weight: 500; }
		.btn:hover { background: #5568d3; }
		.btn-danger { background: #e74c3c; }
		.btn-danger:hover { background: #c0392b; }
		.btn-small { padding: 6px 12px; font-size: 0.9em; }
		.btn-download { background: #27ae60; }
		.btn-download:hover { background: #229954; }
		.clients-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 15px; margin-top: 20px; }
		.client-card { background: #f9f9f9; padding: 15px; border-radius: 4px; border: 1px solid #ddd; }
		.client-card h3 { color: #333; margin-bottom: 10px; }
		.client-card p { color: #666; font-size: 0.9em; margin-bottom: 8px; word-break: break-all; }
		.client-card .actions { margin-top: 10px; display: flex; gap: 5px; flex-wrap: wrap; }
		.error { color: #e74c3c; background: #fadbd8; padding: 10px; border-radius: 4px; margin-bottom: 15px; }
		.success { color: #27ae60; background: #d5f4e6; padding: 10px; border-radius: 4px; margin-bottom: 15px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<div>
				<h1>🌸 Yuki VPN Admin</h1>
				<p>Управление клиентами VPN</p>
			</div>
			<div class="header-right">
				<button class="logout-btn" onclick="logout()">Logout</button>
			</div>
		</div>

		<div class="section">
			<h2>➕ Создать новый клиент</h2>
			<div id="message"></div>
			<div class="form-group">
				<label>Имя клиента</label>
				<input type="text" id="clientName" placeholder="например: client-user-01" />
			</div>
			<div class="form-group">
				<label>Максимальная полоса пропускания (байты)</label>
				<input type="number" id="maxBandwidth" placeholder="1000000" value="1000000" />
			</div>
			<div class="form-group">
				<label>Дата истечения (опционально)</label>
				<input type="date" id="expiresAt" />
			</div>
			<button class="btn" onclick="createClient()">Создать клиента</button>
		</div>

		<div class="section">
			<h2>📱 Активные клиенты</h2>
			<button class="btn" onclick="loadClients()">Обновить список</button>
			<div id="clientsList" class="clients-grid" style="margin-top: 20px;"></div>
		</div>
	</div>

	<script>
		function showMessage(msg, isError) {
			var msgDiv = document.getElementById('message');
			msgDiv.textContent = msg;
			msgDiv.className = isError ? 'error' : 'success';
			setTimeout(function() { msgDiv.textContent = ''; }, 5000);
		}

		function logout() {
			if (confirm('You will be logged out')) {
				document.cookie = 'yuki_session=; path=/admin; max-age=0;';
				location.reload();
			}
		}

		function createClient() {
			var name = document.getElementById('clientName').value;
			var maxBandwidth = parseInt(document.getElementById('maxBandwidth').value) || 1000000;
			var expiresAt = document.getElementById('expiresAt').value || null;

			if (!name) {
				showMessage('Input client name', true);
				return;
			}

			var payload = {
				name: name,
				max_bandwidth: maxBandwidth,
				expires_at: expiresAt ? new Date(expiresAt).toISOString() : null
			};

			fetch('/admin/api/clients', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload)
			})
			.then(function(r) {
				if (r.status === 401) { location.href = '/admin/'; return; }
				return r.json();
			})
			.then(function(data) {
				if (data.id) {
					showMessage('Client created! ID: ' + data.id, false);
					document.getElementById('clientName').value = '';
					document.getElementById('expiresAt').value = '';
					downloadClientConfig(data);
					loadClients();
				} else {
					showMessage('Error: ' + (data.error || 'Unknown'), true);
				}
			})
			.catch(function(e) { showMessage('Network error: ' + e, true); });
		}

		function downloadClientConfig(client) {
			var config = {
				client_id: client.id,
				client_secret: client.secret,
				server_address: location.hostname,
				server_port: 443,
				protocol: 'grpc',
				encryption: 'xchacha20-poly1305',
				created: client.created,
				name: client.name
			};

			var blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
			var url = URL.createObjectURL(blob);
			var a = document.createElement('a');
			a.href = url;
			a.download = 'yuki-' + client.name + '.json';
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
		}

		function loadClients() {
			fetch('/admin/api/clients', { method: 'GET' })
			.then(function(r) {
				if (r.status === 401) { location.href = '/admin/'; return; }
				return r.json();
			})
			.then(function(clients) {
				var list = document.getElementById('clientsList');
				if (!clients || clients.length === 0) {
					list.innerHTML = '<p>No active clients</p>';
					return;
				}

				list.innerHTML = clients.map(function(c) {
					return '<div class="client-card"><h3>Client: ' + c.name + '</h3>' +
						'<p><strong>ID:</strong> ' + c.id + '</p>' +
						'<p><strong>Secret:</strong> ' + c.secret.substring(0, 20) + '...</p>' +
						'<p><strong>Status:</strong> ' + (c.active ? 'Active' : 'Inactive') + '</p>' +
						'<p><strong>Created:</strong> ' + new Date(c.created).toLocaleString() + '</p>' +
						'<p><strong>Traffic:</strong> Up: ' + (c.bytes_up / 1024 / 1024).toFixed(2) + ' MB | Down: ' + (c.bytes_down / 1024 / 1024).toFixed(2) + ' MB</p>' +
						(c.blocked ? '<p style="color: red;"><strong>BLOCKED</strong></p>' : '') +
						'<div class="actions">' +
						'<button class="btn btn-small btn-download" onclick="downloadConfig(\'' + c.id + '\', \'' + c.secret + '\', \'' + c.name + '\')">Download Config</button>' +
						'<button class="btn btn-small" onclick="copyLink(\'' + c.id + '\', \'' + c.secret + '\')">Copy Link</button>' +
						'<button class="btn btn-small" onclick="toggleBlock(\'' + c.id + '\', ' + c.blocked + ')">' + (c.blocked ? 'Unblock' : 'Block') + '</button>' +
						'<button class="btn btn-small btn-danger" onclick="deleteClient(\'' + c.id + '\')">Delete</button>' +
						'</div></div>';
				}).join('');
			})
			.catch(function(e) { showMessage('Load error: ' + e, true); });
		}

		function downloadConfig(clientId, secret, name) {
			var config = {
				client_id: clientId,
				client_secret: secret,
				server_address: location.hostname,
				server_port: 443,
				protocol: 'grpc',
				encryption: 'xchacha20-poly1305',
				name: name
			};

			var blob = new Blob([JSON.stringify(config, null, 2)], { type: 'application/json' });
			var url = URL.createObjectURL(blob);
			var a = document.createElement('a');
			a.href = url;
			a.download = 'yuki-' + name + '.json';
			document.body.appendChild(a);
			a.click();
			document.body.removeChild(a);
			URL.revokeObjectURL(url);
		}

		function deleteClient(clientId) {
			if (!confirm('Delete this client?')) return;

			fetch('/admin/api/clients/' + clientId, { method: 'DELETE' })
			.then(function(r) {
				if (r.status === 401) { location.href = '/admin/'; return; }
				if (r.ok) {
					showMessage('Client deleted', false);
					loadClients();
				} else {
					showMessage('Delete error', true);
				}
			})
			.catch(function(e) { showMessage('Error: ' + e, true); });
		}

		function toggleBlock(clientId, isBlocked) {
			var method = isBlocked ? 'unblock' : 'block';
			fetch('/admin/api/clients/' + clientId + '/' + method, { method: 'POST' })
			.then(function(r) {
				if (r.status === 401) { location.href = '/admin/'; return; }
				if (r.ok) {
					showMessage('Status changed', false);
					loadClients();
				} else {
					showMessage('Error', true);
				}
			})
			.catch(function(e) { showMessage('Error: ' + e, true); });
		}

		function copyLink(clientId, secret) {
			var link = 'yuki://' + clientId + ':' + secret + '@' + location.hostname + ':8443?encryption=xchacha20-poly1305';
			var textarea = document.createElement('textarea');
			textarea.value = link;
			document.body.appendChild(textarea);
			textarea.select();
			document.execCommand('copy');
			document.body.removeChild(textarea);
			showMessage('Link copied to clipboard!', false);
		}

		window.onload = function() {
			loadClients();
		};
	</script>
</body>
</html>`
	fmt.Fprint(w, html)
}

func (a *API) AdminPanelPage(w http.ResponseWriter, r *http.Request) {
	// Check if user has session
	session, _ := r.Cookie("yuki_session")
	if session != nil {
		// User is logged in, show admin panel
		a.AdminPanel(w, r)
		return
	}

	// Show login page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Yuki Admin Login</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); min-height: 100vh; display: flex; align-items: center; justify-content: center; }
		.login-box { background: white; padding: 40px; border-radius: 8px; box-shadow: 0 10px 40px rgba(0,0,0,0.2); width: 100%; max-width: 400px; }
		.login-box h1 { color: #333; margin-bottom: 10px; text-align: center; font-size: 2em; }
		.login-box p { color: #666; text-align: center; margin-bottom: 30px; }
		.form-group { margin-bottom: 20px; }
		.form-group label { display: block; color: #333; font-weight: 500; margin-bottom: 8px; }
		.form-group input { width: 100%; padding: 12px; border: 1px solid #ddd; border-radius: 4px; font-size: 1em; }
		.form-group input:focus { outline: none; border-color: #667eea; box-shadow: 0 0 5px rgba(102, 126, 234, 0.3); }
		.btn { width: 100%; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 12px; border: none; border-radius: 4px; cursor: pointer; font-size: 1em; font-weight: 500; }
		.btn:hover { opacity: 0.9; }
		.error { color: #e74c3c; background: #fadbd8; padding: 12px; border-radius: 4px; margin-bottom: 15px; display: none; }
		.success { color: #27ae60; background: #d5f4e6; padding: 12px; border-radius: 4px; margin-bottom: 15px; display: none; }
	</style>
</head>
<body>
	<div class="login-box">
		<h1>🌸 Yuki VPN</h1>
		<p>Admin Panel</p>
		
		<div id="message"></div>
		
		<form onsubmit="handleLogin(event)">
			<div class="form-group">
				<label>Login</label>
				<input type="text" id="login" required autofocus />
			</div>
			<div class="form-group">
				<label>Password</label>
				<input type="password" id="password" required />
			</div>
			<button type="submit" class="btn">Sign In</button>
		</form>
	</div>

	<script>
		function handleLogin(event) {
			event.preventDefault();
			
			const login = document.getElementById('login').value;
			const password = document.getElementById('password').value;
			const msgDiv = document.getElementById('message');

			fetch('/admin/api/login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ login, password })
			})
			.then(r => {
				if (r.ok) {
					msgDiv.textContent = '✅ Success! Redirecting...';
					msgDiv.style.display = 'block';
					msgDiv.className = 'success';
					setTimeout(() => location.reload(), 1000);
				} else {
					msgDiv.textContent = '❌ Invalid login or password';
					msgDiv.style.display = 'block';
					msgDiv.className = 'error';
				}
			})
			.catch(e => {
				msgDiv.textContent = '❌ Error: ' + e;
				msgDiv.style.display = 'block';
				msgDiv.className = 'error';
			});
		}
	</script>
</body>
</html>`
	fmt.Fprint(w, html)
}

func (a *API) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check credentials
	if req.Login == a.adminLogin && req.Password == a.adminPassword {
		// Set session cookie (valid for 7 days)
		http.SetCookie(w, &http.Cookie{
			Name:     "yuki_session",
			Value:    "authenticated",
			Path:     "/admin",
			MaxAge:   604800, // 7 days
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "invalid credentials"})
}

func (a *API) sessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := r.Cookie("yuki_session")
		if err != nil || session.Value != "authenticated" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
