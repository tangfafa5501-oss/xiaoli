const {test}=require('node:test');
const assert=require('node:assert/strict');
const {shouldReload}=require('../web/build-state.js');
test('reload only a known stale page once per distinct build',()=>{
  assert.equal(shouldReload('A','B',[]),true);
  for(const [page,server,seen] of [['A','A',[]],['A',undefined,[]],[undefined,'B',[]],['__XIAOLI_BUILD__','B',[]],['A','B',['B']]])
    assert.equal(shouldReload(page,server,seen),false);
  assert.equal(shouldReload('A','C',['B']),true);
});

test('426 forces one reload even when status reports the same build',()=>{
 assert.equal(shouldReload('A','A',[],true),true);
 assert.equal(shouldReload('A','A',['A'],true),false);
 assert.equal(shouldReload('__XIAOLI_BUILD__','A',[],true),false);
});
