// codmtune_ui.dll - a tiny integer-ABI shim over GDI+.
//
// Go cannot pass float arguments through syscall.SyscallN on Windows (they
// would land in integer registers), so all GDI+ work lives here. The exported
// ABI uses only ints, pointers and ARGB colors. Build: build_shim.cmd. The DLL
// is embedded into codmtune.exe.

#include <windows.h>
#include <gdiplus.h>
#include <math.h>
#include <string>
#include <vector>

using namespace Gdiplus;

static ULONG_PTR g_token = 0;
static FontFamily* g_family = nullptr;
static FontFamily* g_iconFamily = nullptr;
static bool g_started = false;

struct FontRec {
    int size;
    int bold;
    Font* font;
    StringFormat* fmt;
};
static FontRec g_fonts[32];
static int g_fontN = 0;

struct BrushRec {
    ARGB color;
    SolidBrush* brush;
};
static BrushRec g_brushes[96];
static int g_brushN = 0;

struct PenRec {
    ARGB color;
    int width10;
    Pen* pen;
};
static PenRec g_pens[96];
static int g_penN = 0;

struct Canvas {
    Graphics* g;
    GraphicsState clips[32];
    int clipN;
};

struct ImageRec {
    std::wstring path;
    Image* image;
};
static std::vector<ImageRec> g_images;

static Image* get_image(const WCHAR* path) {
    if (!path) return nullptr;
    for (size_t i = 0; i < g_images.size(); i++)
        if (g_images[i].path == path) return g_images[i].image;
    Image* img = Image::FromFile(path);
    if (!img || img->GetLastStatus() != Ok) {
        delete img;
        return nullptr;
    }
    if (g_images.size() > 8) {
        delete g_images.front().image;
        g_images.erase(g_images.begin());
    }
    ImageRec rec;
    rec.path = path;
    rec.image = img;
    g_images.push_back(rec);
    return img;
}

static SolidBrush* get_brush(ARGB c) {
    for (int i = 0; i < g_brushN; i++)
        if (g_brushes[i].color == c) return g_brushes[i].brush;
    if (g_brushN >= 96) return nullptr;
    SolidBrush* b = new SolidBrush(Color(c));
    g_brushes[g_brushN].color = c;
    g_brushes[g_brushN].brush = b;
    g_brushN++;
    return b;
}

static Pen* get_pen(ARGB c, int width10) {
    for (int i = 0; i < g_penN; i++)
        if (g_pens[i].color == c && g_pens[i].width10 == width10) return g_pens[i].pen;
    if (g_penN >= 96) return nullptr;
    REAL w = width10 / 10.0f;
    if (w < 0.5f) w = 0.5f;
    Pen* p = new Pen(Color(c), w);
    g_pens[g_penN].color = c;
    g_pens[g_penN].width10 = width10;
    g_pens[g_penN].pen = p;
    g_penN++;
    return p;
}

static FontRec* get_font(int size, int bold) {
    for (int i = 0; i < g_fontN; i++)
        if (g_fonts[i].size == size && g_fonts[i].bold == bold) return &g_fonts[i];
    if (g_fontN >= 32 || g_family == nullptr) return nullptr;
    Font* f = new Font(g_family, (REAL)size, bold ? FontStyleBold : FontStyleRegular, UnitPixel);
    if (f->GetLastStatus() != Ok) {
        delete f;
        return nullptr;
    }
    StringFormat* fmt = new StringFormat();
    fmt->SetLineAlignment(StringAlignmentCenter);
    FontRec* r = &g_fonts[g_fontN];
    r->size = size;
    r->bold = bold;
    r->font = f;
    r->fmt = fmt;
    g_fontN++;
    return r;
}

static void make_round_path(GraphicsPath* p, REAL x, REAL y, REAL w, REAL h, REAL rad) {
    if (rad > w / 2) rad = w / 2;
    if (rad > h / 2) rad = h / 2;
    if (rad < 0.5f) {
        p->AddRectangle(RectF(x, y, w, h));
        return;
    }
    REAL d = rad * 2;
    p->AddArc(x, y, d, d, 180, 90);
    p->AddArc(x + w - d, y, d, d, 270, 90);
    p->AddArc(x + w - d, y + h - d, d, d, 0, 90);
    p->AddArc(x, y + h - d, d, d, 90, 90);
    p->CloseFigure();
}

extern "C" {

__declspec(dllexport) int gt_init(void) {
    if (g_started) return 0;
    GdiplusStartupInput in;
    if (GdiplusStartup(&g_token, &in, nullptr) != Ok) return 1;
    const WCHAR* names[4] = {L"Microsoft YaHei UI", L"Microsoft YaHei", L"Segoe UI", L"Arial"};
    for (int i = 0; i < 4 && g_family == nullptr; i++) {
        FontFamily* f = new FontFamily(names[i]);
        if (f->GetLastStatus() == Ok && f->IsAvailable()) g_family = f;
        else delete f;
    }
    g_started = true;
    const WCHAR* icons[2] = {L"Segoe MDL2 Assets", L"Segoe Fluent Icons"};
    for (int i = 0; i < 2 && g_iconFamily == nullptr; i++) {
        FontFamily* f = new FontFamily(icons[i]);
        if (f->GetLastStatus() == Ok && f->IsAvailable()) g_iconFamily = f;
        else delete f;
    }
    return 0;
}

__declspec(dllexport) void gt_shutdown(void) {
    for (size_t i = 0; i < g_images.size(); i++) delete g_images[i].image;
    g_images.clear();
    for (int i = 0; i < g_fontN; i++) {
        delete g_fonts[i].font;
        delete g_fonts[i].fmt;
    }
    g_fontN = 0;
    for (int i = 0; i < g_brushN; i++) delete g_brushes[i].brush;
    g_brushN = 0;
    for (int i = 0; i < g_penN; i++) delete g_pens[i].pen;
    g_penN = 0;
    if (g_family) {
        delete g_family;
        g_family = nullptr;
    }
    if (g_iconFamily) {
        delete g_iconFamily;
        g_iconFamily = nullptr;
    }
    if (g_started) {
        GdiplusShutdown(g_token);
        g_started = false;
    }
}

__declspec(dllexport) void* gt_canvas_new(void* hdc) {
    if (!g_started) return nullptr;
    Canvas* c = new Canvas();
    c->clipN = 0;
    c->g = new Graphics((HDC)hdc);
    if (c->g->GetLastStatus() != Ok) {
        delete c->g;
        delete c;
        return nullptr;
    }
    c->g->SetSmoothingMode(SmoothingModeAntiAlias);
    c->g->SetPixelOffsetMode(PixelOffsetModeHalf);
    // ClearType/GridFit keeps CJK stems and Latin punctuation from dissolving
    // into the paper background at 12–13 px UI sizes. GDI+ falls back to
    // grayscale AA automatically when the target surface cannot do ClearType.
    c->g->SetTextRenderingHint(TextRenderingHintClearTypeGridFit);
    c->g->SetCompositingQuality(CompositingQualityHighQuality);
    c->g->SetInterpolationMode(InterpolationModeHighQualityBicubic);
    return c;
}

__declspec(dllexport) void gt_canvas_free(void* p) {
    Canvas* c = (Canvas*)p;
    if (!c) return;
    while (c->clipN > 0) {
        c->clipN--;
        c->g->Restore(c->clips[c->clipN]);
    }
    delete c->g;
    delete c;
}

__declspec(dllexport) void gt_fill_rect(void* p, int x, int y, int w, int h, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    SolidBrush* b = get_brush((ARGB)argb);
    if (!b) return;
    c->g->FillRectangle(b, x, y, w, h);
}

__declspec(dllexport) void gt_fill_round(void* p, int x, int y, int w, int h, int rad, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    SolidBrush* b = get_brush((ARGB)argb);
    if (!b) return;
    if (rad < 1) {
        c->g->FillRectangle(b, x, y, w, h);
        return;
    }
    GraphicsPath path;
    make_round_path(&path, (REAL)x, (REAL)y, (REAL)w, (REAL)h, (REAL)rad);
    c->g->FillPath(b, &path);
}

__declspec(dllexport) void gt_stroke_round(void* p, int x, int y, int w, int h, int rad, int width10, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    Pen* pen = get_pen((ARGB)argb, width10);
    if (!pen) return;
    REAL inset = width10 / 20.0f;
    GraphicsPath path;
    make_round_path(&path, x + inset, y + inset, w - 2 * inset, h - 2 * inset, (REAL)rad - inset);
    c->g->DrawPath(pen, &path);
}

__declspec(dllexport) void gt_line(void* p, int x1, int y1, int x2, int y2, int width10, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c) return;
    Pen* pen = get_pen((ARGB)argb, width10);
    if (pen) c->g->DrawLine(pen, x1, y1, x2, y2);
}

__declspec(dllexport) void gt_circle(void* p, int cx, int cy, int r, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || r <= 0) return;
    SolidBrush* b = get_brush((ARGB)argb);
    if (b) c->g->FillEllipse(b, cx - r, cy - r, r * 2, r * 2);
}

__declspec(dllexport) void gt_stroke_circle(void* p, int cx, int cy, int r, int width10, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || r <= 0) return;
    Pen* pen = get_pen((ARGB)argb, width10);
    if (pen) c->g->DrawEllipse(pen, cx - r, cy - r, r * 2, r * 2);
}

__declspec(dllexport) void gt_text(void* p, int x, int y, int w, int h, const WCHAR* s, int size, unsigned argb,
                                    int align, int bold, int wrap) {
    Canvas* c = (Canvas*)p;
    if (!c || !s || w <= 0) return;
    FontRec* f = get_font(size, bold);
    if (!f) return;
    SolidBrush* b = get_brush((ARGB)argb);
    if (!b) return;
    f->fmt->SetAlignment((StringAlignment)align);
    f->fmt->SetLineAlignment(StringAlignmentCenter);
    f->fmt->SetTrimming(wrap ? StringTrimmingNone : StringTrimmingEllipsisCharacter);
    RectF rc((REAL)x, (REAL)y, (REAL)w, (REAL)h);
    c->g->DrawString(s, -1, f->font, rc, f->fmt, b);
}

__declspec(dllexport) void gt_measure(void* p, const WCHAR* s, int size, int bold, int* outW, int* outH) {
    Canvas* c = (Canvas*)p;
    *outW = 0;
    *outH = 0;
    if (!c || !s) return;
    FontRec* f = get_font(size, bold);
    if (!f) return;
    f->fmt->SetAlignment(StringAlignmentNear);
    f->fmt->SetLineAlignment(StringAlignmentNear);
    f->fmt->SetTrimming(StringTrimmingNone);
    RectF rc(0, 0, 10000, (REAL)size * 2);
    RectF bound;
    if (c->g->MeasureString(s, -1, f->font, rc, f->fmt, &bound) == Ok) {
        *outW = (int)ceilf(bound.Width);
        *outH = (int)ceilf(bound.Height);
    }
}

__declspec(dllexport) void gt_push_clip(void* p, int x, int y, int w, int h) {
    Canvas* c = (Canvas*)p;
    if (!c || c->clipN >= 32) return;
    c->clips[c->clipN++] = c->g->Save();
    c->g->SetClip(Rect(x, y, w, h), CombineModeIntersect);
}

__declspec(dllexport) void gt_pop_clip(void* p) {
    Canvas* c = (Canvas*)p;
    if (!c || c->clipN <= 0) return;
    c->clipN--;
    c->g->Restore(c->clips[c->clipN]);
}

__declspec(dllexport) void gt_set_view(void* p, int milli, int ox, int oy) {
    Canvas* c = (Canvas*)p;
    if (!c || milli <= 0) return;
    REAL s = milli / 1000.0f;
    c->g->ResetTransform();
    c->g->ScaleTransform(s, s);
    c->g->TranslateTransform((REAL)ox, (REAL)oy);
}

__declspec(dllexport) void gt_set_scale(void* p, int milli) {
    Canvas* c = (Canvas*)p;
    if (!c || milli <= 0) return;
    REAL s = milli / 1000.0f;
    c->g->ResetTransform();
    c->g->ScaleTransform(s, s);
}

__declspec(dllexport) void gt_glyph(void* p, const WCHAR* s, int x, int y, int w, int h, int size, unsigned argb) {
    Canvas* c = (Canvas*)p;
    if (!c || !s || !g_iconFamily || w <= 0 || h <= 0) return;
    SolidBrush* b = get_brush((ARGB)argb);
    if (!b) return;
    Font f(g_iconFamily, (REAL)size, FontStyleRegular, UnitPixel);
    StringFormat fmt;
    fmt.SetAlignment(StringAlignmentCenter);
    fmt.SetLineAlignment(StringAlignmentCenter);
    fmt.SetTrimming(StringTrimmingNone);
    RectF rc((REAL)x, (REAL)y, (REAL)w, (REAL)h);
    c->g->DrawString(s, -1, &f, rc, &fmt, b);
}

__declspec(dllexport) int gt_version(void) { return 4; }

__declspec(dllexport) int gt_set_font_family(const WCHAR* name) {
    if (!g_started || !name) return 1;
    FontFamily* f = new FontFamily(name);
    if (f->GetLastStatus() != Ok || !f->IsAvailable()) {
        delete f;
        return 2;
    }
    for (int i = 0; i < g_fontN; i++) {
        delete g_fonts[i].font;
        delete g_fonts[i].fmt;
    }
    g_fontN = 0;
    if (g_family) delete g_family;
    g_family = f;
    return 0;
}

__declspec(dllexport) void gt_image_size(const WCHAR* path, int* w, int* h) {
    *w = 0;
    *h = 0;
    Image* img = get_image(path);
    if (!img) return;
    *w = (int)img->GetWidth();
    *h = (int)img->GetHeight();
}

__declspec(dllexport) void gt_draw_image(void* p, const WCHAR* path, int x, int y, int w, int h, int alpha) {
    Canvas* c = (Canvas*)p;
    if (!c || w <= 0 || h <= 0) return;
    Image* img = get_image(path);
    if (!img) return;
    if (alpha >= 255) {
        c->g->DrawImage(img, Rect(x, y, w, h), 0, 0, (INT)img->GetWidth(), (INT)img->GetHeight(), UnitPixel);
        return;
    }
    ImageAttributes attr;
    ColorMatrix cm = {
        1,0,0,0,0,
        0,1,0,0,0,
        0,0,1,0,0,
        0,0,0, (REAL)alpha/255.0f, 0,
        0,0,0,0,1
    };
    attr.SetColorMatrix(&cm, ColorMatrixFlagsDefault, ColorAdjustTypeBitmap);
    c->g->DrawImage(img, Rect(x, y, w, h), 0, 0, (INT)img->GetWidth(), (INT)img->GetHeight(), UnitPixel, &attr);
}

} // extern "C"

BOOL WINAPI DllMain(HINSTANCE, DWORD, LPVOID) { return TRUE; }
