using System;
using System.Drawing;
using System.Drawing.Imaging;

namespace WebratCs.Desktop
{
    public static class DesktopCapture
    {
        public static byte[] CaptureDisplayJpegScaled(int display, int quality, int width, int height)
        {
            var screens = System.Windows.Forms.Screen.AllScreens;
            if (display < 0 || display >= screens.Length) display = 0;
            Rectangle bounds = screens[display].Bounds;
            using (var bmp = new Bitmap(bounds.Width, bounds.Height, PixelFormat.Format24bppRgb))
            using (var g = Graphics.FromImage(bmp))
            {
                g.CopyFromScreen(bounds.Left, bounds.Top, 0, 0, bounds.Size, CopyPixelOperation.SourceCopy);
                Image src = bmp;
                if (width > 0 && height > 0)
                {
                    var dst = new Bitmap(width, height, PixelFormat.Format24bppRgb);
                    using (var gg = Graphics.FromImage(dst))
                    {
                        gg.InterpolationMode = System.Drawing.Drawing2D.InterpolationMode.HighQualityBicubic;
                        gg.DrawImage(src, 0, 0, width, height);
                    }
                    src = dst;
                }
                using (var ms = new System.IO.MemoryStream())
                {
                    var enc = GetJpegEncoder();
                    var ep = new EncoderParameters(1);
                    ep.Param[0] = new EncoderParameter(System.Drawing.Imaging.Encoder.Quality, Math.Max(10, Math.Min(quality, 95)));
                    src.Save(ms, enc, ep);
                    return ms.ToArray();
                }
            }
        }
        public static byte[] CaptureJpegScaled(int quality, int width, int height)
        {
            Rectangle bounds = System.Windows.Forms.Screen.PrimaryScreen.Bounds;
            using (var bmp = new Bitmap(bounds.Width, bounds.Height, PixelFormat.Format24bppRgb))
            using (var g = Graphics.FromImage(bmp))
            {
                g.CopyFromScreen(bounds.Left, bounds.Top, 0, 0, bounds.Size, CopyPixelOperation.SourceCopy);
                Image src = bmp;
                if (width > 0 && height > 0)
                {
                    var dst = new Bitmap(width, height, PixelFormat.Format24bppRgb);
                    using (var gg = Graphics.FromImage(dst))
                    {
                        gg.InterpolationMode = System.Drawing.Drawing2D.InterpolationMode.HighQualityBicubic;
                        gg.DrawImage(src, 0, 0, width, height);
                    }
                    src = dst;
                }
                using (var ms = new System.IO.MemoryStream())
                {
                    var enc = GetJpegEncoder();
                    var ep = new EncoderParameters(1);
                    ep.Param[0] = new EncoderParameter(System.Drawing.Imaging.Encoder.Quality, Math.Max(10, Math.Min(quality, 95)));
                    src.Save(ms, enc, ep);
                    return ms.ToArray();
                }
            }
        }

        private static ImageCodecInfo GetJpegEncoder()
        {
            foreach (var c in ImageCodecInfo.GetImageEncoders())
            {
                if (c.MimeType.Equals("image/jpeg", StringComparison.OrdinalIgnoreCase)) return c;
            }
            return ImageCodecInfo.GetImageEncoders()[0];
        }
    }
}
