//! Turn-scoped ownership for terminals on Windows without nested Job Objects.

use super::UnifiedExecProcessManager;
use super::process_manager::unregister_network_approval_for_entry;

impl UnifiedExecProcessManager {
    pub(crate) async fn terminate_turn_processes(&self, turn_id: &str) {
        let entries = {
            let mut store = self.process_store.lock().await;
            // Snapshot IDs before remove() mutates both the process map and its
            // reserved-ID set; a single iterator would borrow the store twice.
            #[allow(clippy::needless_collect)]
            let ids = store
                .processes
                .iter()
                .filter_map(|(id, entry)| (entry.turn_id == turn_id).then_some(*id))
                .collect::<Vec<_>>();
            ids.into_iter()
                .filter_map(|id| store.remove(id))
                .collect::<Vec<_>>()
        };
        for entry in entries {
            unregister_network_approval_for_entry(&entry).await;
            entry.process.terminate();
        }
    }
}
