(function(root) {
  function shouldReload(pageBuild, serverBuild, seen, force=false) {
    return !!pageBuild && !pageBuild.includes('__XIAOLI_') && !!serverBuild &&
      (force || pageBuild !== serverBuild) && !seen.includes(serverBuild);
  }
  if(typeof module !== 'undefined') module.exports={shouldReload};
  else root.XiaoliBuild={shouldReload};
})(globalThis);
