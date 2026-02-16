using System;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;

namespace WebratCs.Camera
{
    public class AvicapCamera : IDisposable
    {
        private IntPtr hwnd = IntPtr.Zero;
        private int width;
        private int height;
        private int fps;
        private int index;
        public static string[] ListDevices()
        {
            var list = new System.Collections.Generic.List<string>();
            for (int i = 0; i < 20; i++)
            {
                var nameBuf = new byte[256];
                var verBuf = new byte[256];
                var ok = capGetDriverDescriptionA(i, nameBuf, nameBuf.Length, verBuf, verBuf.Length);
                if (ok)
                {
                    int n = Array.IndexOf(nameBuf, (byte)0);
                    if (n < 0) n = nameBuf.Length;
                    var nm = System.Text.Encoding.ASCII.GetString(nameBuf, 0, n).Trim();
                    if (!string.IsNullOrEmpty(nm)) list.Add(nm);
                }
            }
            return list.ToArray();
        }

        public AvicapCamera(string deviceName, int w, int h, int fps)
        {
            this.width = w > 0 ? w : 640;
            this.height = h > 0 ? h : 480;
            this.fps = fps > 0 ? fps : 10;
            this.index = FindDeviceIndex(deviceName);
            hwnd = capCreateCaptureWindowA("cap", 0, 0, 0, this.width, this.height, IntPtr.Zero, 0);
            if (hwnd == IntPtr.Zero) throw new Exception("capCreateCaptureWindowA failed");
            if (SendMessage(hwnd, WM_CAP_DRIVER_CONNECT, (IntPtr)index, IntPtr.Zero) == IntPtr.Zero)
            {
                throw new Exception("WM_CAP_DRIVER_CONNECT failed");
            }
            SendMessage(hwnd, WM_CAP_SET_SCALE, (IntPtr)1, IntPtr.Zero);
            SendMessage(hwnd, WM_CAP_SET_PREVIEW, (IntPtr)1, IntPtr.Zero);
            SendMessage(hwnd, WM_CAP_SET_PREVIEWRATE, (IntPtr)Math.Max(50, 1000 / this.fps), IntPtr.Zero);
            System.Threading.Thread.Sleep(100);
        }

        public byte[] CaptureJpeg(int quality)
        {
            var bmp = CaptureBitmap();
            if (bmp == null) throw new Exception("capture failed");
            using (bmp)
            using (var ms = new System.IO.MemoryStream())
            {
                var enc = GetJpegEncoder();
                var ep = new EncoderParameters(1);
                ep.Param[0] = new EncoderParameter(System.Drawing.Imaging.Encoder.Quality, Math.Max(10, Math.Min(quality, 95)));
                bmp.Save(ms, enc, ep);
                return ms.ToArray();
            }
        }

        private Bitmap CaptureBitmap()
        {
            RECT rc;
            GetClientRect(hwnd, out rc);
            int w = rc.right - rc.left;
            int h = rc.bottom - rc.top;
            IntPtr hdc = GetDC(hwnd);
            if (hdc == IntPtr.Zero) return null;
            IntPtr memDC = CreateCompatibleDC(hdc);
            if (memDC == IntPtr.Zero) { ReleaseDC(hwnd, hdc); return null; }
            IntPtr hbm = CreateCompatibleBitmap(hdc, w, h);
            if (hbm == IntPtr.Zero) { DeleteDC(memDC); ReleaseDC(hwnd, hdc); return null; }
            IntPtr prev = SelectObject(memDC, hbm);
            SendMessage(hwnd, WM_CAP_GRAB_FRAME, IntPtr.Zero, IntPtr.Zero);
            IntPtr pr = PrintWindow(hwnd, memDC, (IntPtr)PW_RENDERFULLCONTENT);
            if (pr == IntPtr.Zero)
            {
                BitBlt(memDC, 0, 0, w, h, hdc, 0, 0, SRCCOPY);
            }
            Bitmap bmp = Image.FromHbitmap(hbm);
            SelectObject(memDC, prev);
            DeleteObject(hbm);
            DeleteDC(memDC);
            ReleaseDC(hwnd, hdc);
            return bmp;
        }

        public void Dispose()
        {
            try
            {
                if (hwnd != IntPtr.Zero)
                {
                    SendMessage(hwnd, WM_CAP_DRIVER_DISCONNECT, IntPtr.Zero, IntPtr.Zero);
                    DestroyWindow(hwnd);
                    hwnd = IntPtr.Zero;
                }
            }
            catch { }
        }

        private int FindDeviceIndex(string name)
        {
            for (int i = 0; i < 20; i++)
            {
                var nameBuf = new byte[256];
                var verBuf = new byte[256];
                var ok = capGetDriverDescriptionA(i, nameBuf, nameBuf.Length, verBuf, verBuf.Length);
                if (ok)
                {
                    int n = Array.IndexOf(nameBuf, (byte)0);
                    if (n < 0) n = nameBuf.Length;
                    var nm = System.Text.Encoding.ASCII.GetString(nameBuf, 0, n).Trim();
                    var tgt = name == null ? "" : name.Trim();
                    if (string.Equals(nm, tgt, StringComparison.OrdinalIgnoreCase)) return i;
                }
            }
            return 0;
        }

        private static ImageCodecInfo GetJpegEncoder()
        {
            foreach (var c in ImageCodecInfo.GetImageEncoders())
            {
                if (c.MimeType.Equals("image/jpeg", StringComparison.OrdinalIgnoreCase)) return c;
            }
            return ImageCodecInfo.GetImageEncoders()[0];
        }

        // PInvoke
        private const int WM_CAP_START = 0x400;
        private static readonly int WM_CAP_DRIVER_CONNECT = WM_CAP_START + 10;
        private static readonly int WM_CAP_DRIVER_DISCONNECT = WM_CAP_START + 11;
        private static readonly int WM_CAP_SET_PREVIEW = WM_CAP_START + 50;
        private static readonly int WM_CAP_SET_PREVIEWRATE = WM_CAP_START + 52;
        private static readonly int WM_CAP_SET_SCALE = WM_CAP_START + 53;
        private static readonly int WM_CAP_GRAB_FRAME = WM_CAP_START + 60;
        private static readonly int SRCCOPY = 0x00CC0020;
        private static readonly int PW_CLIENTONLY = 0x00000001;
        private static readonly int PW_RENDERFULLCONTENT = 0x00000002;

        [StructLayout(LayoutKind.Sequential)]
        private struct RECT { public int left, top, right, bottom; }

        [DllImport("avicap32.dll", CharSet = CharSet.Ansi)]
        private static extern IntPtr capCreateCaptureWindowA(string lpszName, int dwStyle, int X, int Y, int nWidth, int nHeight, IntPtr hwndParent, int nID);

        [DllImport("avicap32.dll", CharSet = CharSet.Ansi)]
        private static extern bool capGetDriverDescriptionA(int wDriverIndex, byte[] lpszName, int cbName, byte[] lpszVer, int cbVer);

        [DllImport("user32.dll")]
        private static extern IntPtr SendMessage(IntPtr hWnd, int Msg, IntPtr wParam, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern bool DestroyWindow(IntPtr hWnd);

        [DllImport("user32.dll")]
        private static extern IntPtr GetDC(IntPtr hWnd);

        [DllImport("user32.dll")]
        private static extern int ReleaseDC(IntPtr hWnd, IntPtr hDC);

        [DllImport("user32.dll")]
        private static extern bool GetClientRect(IntPtr hWnd, out RECT lpRect);

        [DllImport("user32.dll")]
        private static extern IntPtr PrintWindow(IntPtr hwnd, IntPtr hdcBlt, IntPtr nFlags);

        [DllImport("gdi32.dll")]
        private static extern IntPtr CreateCompatibleDC(IntPtr hdc);

        [DllImport("gdi32.dll")]
        private static extern bool DeleteDC(IntPtr hdc);

        [DllImport("gdi32.dll")]
        private static extern IntPtr CreateCompatibleBitmap(IntPtr hdc, int cx, int cy);

        [DllImport("gdi32.dll")]
        private static extern IntPtr SelectObject(IntPtr hdc, IntPtr h);

        [DllImport("gdi32.dll")]
        private static extern bool BitBlt(IntPtr hdc, int x, int y, int cx, int cy, IntPtr hdcSrc, int x1, int y1, int rop);

        [DllImport("gdi32.dll")]
        private static extern bool DeleteObject(IntPtr hObject);
    }
}
