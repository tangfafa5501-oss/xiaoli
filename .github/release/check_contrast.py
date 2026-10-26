"""WCAG 2.2 relative luminance; worst wallpaper = opaque white.
Run directly. Shadow and blur are deliberately excluded from the guarantee.
"""
import json

def luminance(rgb):
    def linear(c):
        c /= 255
        return c / 12.92 if c <= 0.04045 else ((c + .055) / 1.055) ** 2.4
    return sum(w * linear(c) for w, c in zip((.2126, .7152, .0722), rgb))

def contrast(fg, bg):
    a, b = sorted((luminance(fg), luminance(bg)))
    return (b + .05) / (a + .05)

if __name__ == '__main__':
    glass = tuple(.8 * c + .2 * 255 for c in (20, 24, 36))
    rows = []
    for name, fg, bg in [
        ('white/glass', (255,255,255), glass),
        ('name/glass', (255,214,170), glass),
        ('partial,date,placeholder,status/glass', (203,213,225), glass),
        ('white/fallback', (255,255,255), (20,24,36)),
        ('name/fallback', (255,214,170), (20,24,36)),
        ('partial/fallback', (203,213,225), (20,24,36)),
        ('send', (255,255,255), (135,66,31)),
        ('send disabled', (203,213,225), (82,56,43)),
        ('mic listening', (255,255,255), (135,60,48)),
        ('mic icon', (150,96,60), (250,240,228)),
    ]:
        ratio = contrast(fg, bg)
        assert ratio >= 4.5, (name, ratio)
        rows.append({'name':name, 'contrast':round(ratio,6)})
    print(json.dumps({'glass_rgb':glass, 'glass_luminance':luminance(glass), 'ratios':rows}, indent=2))
