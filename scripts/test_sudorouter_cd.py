# sudoapi: Guarded single-VM SudoRouter continuous deployment.
import json
import unittest

from sudorouter_cd import completed_release


class ReleaseReceiptTests(unittest.TestCase):
    def result(self, **values):
        return {'value': [{'message': '[stdout]\nSUDOROUTER_RESULT=' + json.dumps(values) + '\n[stderr]'}]}

    def test_vm_success_without_a_completed_rollout_is_rejected(self):
        for result in [{}, self.result(phase='manual_recovery_required'), self.result(phase='rolled_back'),
                       self.result(phase='succeeded'),
                       self.result(phase='succeeded', bridge_removed=True, availability_failures=1)]:
            with self.subTest(result=result):
                with self.assertRaises(RuntimeError):
                    completed_release(result)

    def test_completed_cleanup_and_already_current_are_accepted(self):
        for receipt in [dict(phase='succeeded', bridge_removed=True, availability_failures=0),
                        dict(phase='succeeded', already_running=True)]:
            self.assertEqual(completed_release(self.result(**receipt)), receipt)


if __name__ == '__main__':
    unittest.main()
