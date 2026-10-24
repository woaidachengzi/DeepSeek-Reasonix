"""The acceptance model must fail when coordinate clicks do nothing."""
import json
import pathlib
import tempfile
import unittest

from provider import FixtureModel


class CoordinateClickAcceptanceTest(unittest.TestCase):
    def test_coordinate_button_is_independent_and_field_requires_two_clicks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            fixture = {'pid': 42, 'clicks': 1, 'text': ''}
            (root / 'fixture-state.json').write_text(json.dumps(fixture))
            state = {'bounds': {'x': -1920, 'y': 30}, 'elements': [
                {'element': '0.1', 'role': 'AXButton', 'title': 'Increment test counter',
                 'bounds': {'x': -1800, 'y': 100, 'width': 100, 'height': 40}},
                {'element': '0.2', 'role': 'AXTextField',
                 'bounds': {'x': -1800, 'y': 200, 'width': 300, 'height': 30}},
            ]}
            request = {'tools': [{'function': {'name': 'use_capability'}}], 'messages': [
                {'role': 'tool', 'content': json.dumps({'state': state, 'observation': 'receipt'})},
            ]}
            model = FixtureModel(root)
            model.step = model.actions.index('click_button_coordinate')
            call = model.reply(request)['tool_calls'][0]
            args = json.loads(call['function']['arguments'])['arguments']
            self.assertEqual((args['action'], args['x'], args['y']), ('click', 170, 90))
            self.assertNotIn('element', args, 'AX press must not stand in for coordinate acceptance')
            model.step = model.actions.index('click_field')
            reply = model.reply(request)
            self.assertNotIn('tool_calls', reply)
            self.assertIn('coordinate-button click postcondition failed', reply['content'])
            fixture['clicks'] = 2
            (root / 'fixture-state.json').write_text(json.dumps(fixture))
            model.step = model.actions.index('click_field')
            self.assertIn('tool_calls', model.reply(request))


if __name__ == '__main__':
    unittest.main()
