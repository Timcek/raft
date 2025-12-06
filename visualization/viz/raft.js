'use strict';

var raft = {};

raft.server = function(id, peers) {
  return {
    id: id,
    peers: peers,
    state: 'follower',
    term: 0,
    votedFor: null,
    log: [],
    commitIndex: 0,
    electionAlarm: 0,
    voteGranted:  util.makeMap(peers, false),
    matchIndex:   util.makeMap(peers, 0),
    nextIndex:    util.makeMap(peers, 1),
    rpcDue:       util.makeMap(peers, 0),
    heartbeatDue: util.makeMap(peers, 0),
  };
};

raft.update = function(model) {
  var deliver = [];
  var keep = [];
  model.messages.forEach(function(message) {
    if (message.recvTime <= model.time)
      deliver.push(message);
    else if (message.recvTime < util.Inf)
      keep.push(message);
  });
  model.messages = keep;
};

raft.clientRequest = function(model, server) {
  if (server.state === 'leader') {
    ws[server.id-1].send(JSON.stringify({method: "request", message: "default"}));
  }
};

raft.stop = function() {
  playback.pause()
};

raft.resumeAll = function() {
  playback.resume()
};