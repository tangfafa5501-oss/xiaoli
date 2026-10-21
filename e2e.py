import re,sys,time
from playwright.sync_api import sync_playwright
url=sys.argv[1]
qs=["你好，你是谁？","帮我写一条请假消息，明天家里有事","这条短信是不是骗子：【顺丰】您的快递丢失，点击链接 http://sf-pay.cc 办理理赔，需提供银行卡和验证码","那我刚才说明天要干嘛来着？"]
with sync_playwright() as p:
    b=p.chromium.launch(executable_path='/usr/bin/google-chrome',args=['--no-sandbox'])
    pg=b.new_page(viewport={'width':480,'height':860})
    t=time.time(); pg.goto(url)
    pg.wait_for_function("document.querySelectorAll('.msg.her').length>=1 && !document.getElementById('send').disabled",timeout=120000)
    print('GREETING (%.1fs):'%(time.time()-t), pg.inner_text('.msg.her'))
    pg.screenshot(path='shot_greeting.png')
    for i,q in enumerate(qs):
        n=pg.locator('.msg.her').count()
        t=time.time()
        if i==0: pg.click('.chip >> nth=0') if False else None
        pg.fill('#input',q); pg.press('#input','Enter')
        pg.wait_for_function(f"document.querySelectorAll('.msg.her').length>{n} && !document.getElementById('send').disabled",timeout=120000)
        a=pg.locator('.msg.her').nth(n).inner_text()
        print(f'\nQ: {q}\nA ({time.time()-t:.1f}s): {a}\n  think-leak: {"think" in a.lower()}')
    pg.screenshot(path='shot_chat.png')
    b.close()
